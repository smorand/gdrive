// Package mcp implements the MCP HTTP Streamable server for Google Drive.
package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
)

const (
	stateTTL = 10 * time.Minute
	codeTTL  = 10 * time.Minute
)

// OAuthCredentials holds the Google OAuth client credentials.
type OAuthCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// registeredClient represents an OAuth2 dynamically registered client.
type registeredClient struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
	CreatedAt    time.Time
}

// authState represents an in-flight OAuth2 authorization state.
type authState struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	CodeMethod    string
	ClientState   string
	CreatedAt     time.Time
}

// authCode represents an issued authorization code.
type authCode struct {
	Code          string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	CodeMethod    string
	GoogleToken   *oauth2.Token
	CreatedAt     time.Time
}

// OAuth2Server implements an OAuth 2.1 authorization server that proxies to Google OAuth.
type OAuth2Server struct {
	baseURL     string
	oauthConfig *oauth2.Config

	mu      sync.RWMutex
	clients map[string]*registeredClient
	states  map[string]*authState
	codes   map[string]*authCode
}

// NewOAuth2Server creates a new OAuth2 authorization server.
func NewOAuth2Server(baseURL string, creds *OAuthCredentials) *OAuth2Server {
	oauthConfig := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Scopes:       []string{drive.DriveScope, driveactivity.DriveActivityReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	srv := &OAuth2Server{
		baseURL:     strings.TrimRight(baseURL, "/"),
		oauthConfig: oauthConfig,
		clients:     make(map[string]*registeredClient),
		states:      make(map[string]*authState),
		codes:       make(map[string]*authCode),
	}

	go srv.cleanupLoop()
	return srv
}

// cleanupLoop periodically removes expired states and codes.
func (s *OAuth2Server) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.states {
			if now.Sub(v.CreatedAt) > stateTTL {
				delete(s.states, k)
			}
		}
		for k, v := range s.codes {
			if now.Sub(v.CreatedAt) > codeTTL {
				delete(s.codes, k)
			}
		}
		s.mu.Unlock()
	}
}

// GetOAuthConfig returns the OAuth2 config for use by auth middleware.
func (s *OAuth2Server) GetOAuthConfig() *oauth2.Config {
	return s.oauthConfig
}

// HandleProtectedResourceMetadata implements RFC 9728.
func (s *OAuth2Server) HandleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]interface{}{
		"resource":                 s.baseURL,
		"authorization_servers":    []string{s.baseURL},
		"bearer_methods_supported": []string{"header"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// HandleAuthorizationServerMetadata implements RFC 8414.
func (s *OAuth2Server) HandleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]interface{}{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/oauth/authorize",
		"token_endpoint":                        s.baseURL + "/oauth/token",
		"registration_endpoint":                 s.baseURL + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// HandleClientRegistration implements RFC 7591 Dynamic Client Registration.
func (s *OAuth2Server) HandleClientRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	client := s.registerClient(req.RedirectURIs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"client_id":     client.ClientID,
		"client_secret": client.ClientSecret,
		"redirect_uris": client.RedirectURIs,
	})
}

// HandleAuthorize handles the authorization endpoint.
func (s *OAuth2Server) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeMethod := r.URL.Query().Get("code_challenge_method")
	state := r.URL.Query().Get("state")

	if clientID == "" || redirectURI == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_id and redirect_uri are required")
		return
	}

	// Auto-register unknown clients
	s.mu.RLock()
	_, exists := s.clients[clientID]
	s.mu.RUnlock()
	if !exists {
		s.registerClientWithID(clientID, []string{redirectURI})
		slog.Info("auto-registered client", "client_id", clientID)
	}

	// Generate internal state
	internalState := generateToken()

	s.mu.Lock()
	s.states[internalState] = &authState{
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		CodeChallenge: codeChallenge,
		CodeMethod:    codeMethod,
		ClientState:   state,
		CreatedAt:     time.Now(),
	}
	s.mu.Unlock()

	// Build Google OAuth URL
	s.oauthConfig.RedirectURL = s.baseURL + "/oauth/callback"
	authURL := s.oauthConfig.AuthCodeURL(internalState,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	slog.Info("redirecting to Google OAuth", "client_id", clientID)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles the Google OAuth callback.
func (s *OAuth2Server) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Check for error from Google
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		slog.Error("OAuth callback error from Google", "error", errParam)
		writeOAuthError(w, http.StatusBadRequest, errParam, "Authorization denied by user")
		return
	}

	code := r.URL.Query().Get("code")
	internalState := r.URL.Query().Get("state")

	if code == "" || internalState == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing code or state")
		return
	}

	// Look up the stored state
	s.mu.Lock()
	storedState, exists := s.states[internalState]
	if exists {
		delete(s.states, internalState)
	}
	s.mu.Unlock()

	if !exists {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid or expired state")
		return
	}

	// Check state TTL
	if time.Since(storedState.CreatedAt) > stateTTL {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid or expired state")
		return
	}

	// Exchange code with Google
	s.oauthConfig.RedirectURL = s.baseURL + "/oauth/callback"
	googleToken, err := s.oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		slog.Error("failed to exchange code with Google", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to exchange authorization code")
		return
	}

	if googleToken.RefreshToken == "" {
		slog.Warn("no refresh token received from Google")
	}

	// Generate our own authorization code
	ourCode := generateToken()

	s.mu.Lock()
	s.codes[ourCode] = &authCode{
		Code:          ourCode,
		ClientID:      storedState.ClientID,
		RedirectURI:   storedState.RedirectURI,
		CodeChallenge: storedState.CodeChallenge,
		CodeMethod:    storedState.CodeMethod,
		GoogleToken:   googleToken,
		CreatedAt:     time.Now(),
	}
	s.mu.Unlock()

	// Redirect back to client with our authorization code
	redirectURL := storedState.RedirectURI
	sep := "?"
	if strings.Contains(redirectURL, "?") {
		sep = "&"
	}
	redirectURL += sep + "code=" + ourCode
	if storedState.ClientState != "" {
		redirectURL += "&state=" + storedState.ClientState
	}

	slog.Info("OAuth callback successful, redirecting to client", "client_id", storedState.ClientID)

	// Show success page for browser flows, redirect for programmatic flows
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Authorization Successful</title></head>
<body>
<h1>Authorization Successful!</h1>
<p>You can close this window. Redirecting...</p>
<script>window.location.href = %q;</script>
</body></html>`, redirectURL)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleToken handles the token endpoint.
func (s *OAuth2Server) HandleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid form data")
		return
	}

	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleAuthorizationCodeGrant(w, r)
	case "refresh_token":
		s.handleRefreshTokenGrant(w, r)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Unsupported grant type: "+grantType)
	}
}

func (s *OAuth2Server) handleAuthorizationCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")

	if code == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing code")
		return
	}

	// Look up and consume the authorization code
	s.mu.Lock()
	storedCode, exists := s.codes[code]
	if exists {
		delete(s.codes, code)
	}
	s.mu.Unlock()

	if !exists {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Invalid or expired authorization code")
		return
	}

	// Check code TTL
	if time.Since(storedCode.CreatedAt) > codeTTL {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Invalid or expired authorization code")
		return
	}

	// Validate PKCE if code_challenge was provided
	if storedCode.CodeChallenge != "" {
		if codeVerifier == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Missing code_verifier")
			return
		}
		if !validatePKCE(codeVerifier, storedCode.CodeChallenge, storedCode.CodeMethod) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Invalid code_verifier")
			return
		}
	}

	// Return the Google tokens to the client
	response := map[string]interface{}{
		"access_token": storedCode.GoogleToken.AccessToken,
		"token_type":   "Bearer",
	}

	if storedCode.GoogleToken.RefreshToken != "" {
		response["refresh_token"] = storedCode.GoogleToken.RefreshToken
	}

	if !storedCode.GoogleToken.Expiry.IsZero() {
		expiresIn := int(time.Until(storedCode.GoogleToken.Expiry).Seconds())
		if expiresIn > 0 {
			response["expires_in"] = expiresIn
		}
	}

	slog.Info("token exchange successful", "client_id", storedCode.ClientID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *OAuth2Server) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	if refreshToken == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing refresh_token")
		return
	}

	// Use the refresh token to get a new access token from Google
	token := &oauth2.Token{RefreshToken: refreshToken}
	tokenSource := s.oauthConfig.TokenSource(context.Background(), token)
	newToken, err := tokenSource.Token()
	if err != nil {
		slog.Error("failed to refresh token", "error", err)
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Failed to refresh token")
		return
	}

	response := map[string]interface{}{
		"access_token": newToken.AccessToken,
		"token_type":   "Bearer",
	}

	if newToken.RefreshToken != "" {
		response["refresh_token"] = newToken.RefreshToken
	}

	if !newToken.Expiry.IsZero() {
		expiresIn := int(time.Until(newToken.Expiry).Seconds())
		if expiresIn > 0 {
			response["expires_in"] = expiresIn
		}
	}

	slog.Info("token refresh successful")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ValidateAccessToken validates an access token and returns an OAuth config and token for API calls.
func (s *OAuth2Server) ValidateAccessToken(accessToken string) (*oauth2.Config, *oauth2.Token, error) {
	if accessToken == "" {
		return nil, nil, fmt.Errorf("empty access token")
	}

	token := &oauth2.Token{AccessToken: accessToken}
	return s.oauthConfig, token, nil
}

// registerClient creates a new registered client.
func (s *OAuth2Server) registerClient(redirectURIs []string) *registeredClient {
	clientID := generateToken()
	return s.registerClientWithID(clientID, redirectURIs)
}

func (s *OAuth2Server) registerClientWithID(clientID string, redirectURIs []string) *registeredClient {
	client := &registeredClient{
		ClientID:     clientID,
		ClientSecret: generateToken(),
		RedirectURIs: redirectURIs,
		CreatedAt:    time.Now(),
	}

	s.mu.Lock()
	s.clients[clientID] = client
	s.mu.Unlock()

	return client
}

// LoadOAuthCredentials loads Google OAuth credentials from Secret Manager or local file.
func LoadOAuthCredentials(secretName, secretProject, credentialFile string) (*OAuthCredentials, error) {
	// Try Secret Manager first
	if secretName != "" && secretProject != "" {
		creds, err := loadFromSecretManager(secretName, secretProject)
		if err != nil {
			slog.Warn("failed to load credentials from Secret Manager, trying local file",
				"error", err, "secret_name", secretName)
		} else {
			slog.Info("loaded OAuth credentials from Secret Manager", "secret_name", secretName)
			return creds, nil
		}
	}

	// Fall back to local file
	if credentialFile != "" {
		return loadFromFile(credentialFile)
	}

	// Try default locations
	for _, path := range []string{"credentials.json", "google_credentials.json"} {
		if _, err := os.Stat(path); err == nil {
			return loadFromFile(path)
		}
	}

	return nil, fmt.Errorf("no OAuth credentials found: set --secret-name/--secret-project or --credential-file")
}

func loadFromSecretManager(secretName, projectID string) (*OAuthCredentials, error) {
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create secret manager client: %w", err)
	}
	defer client.Close()

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretName)
	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("access secret %s: %w", name, err)
	}

	return parseCredentials(result.Payload.Data)
}

func loadFromFile(path string) (*OAuthCredentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credential file %s: %w", path, err)
	}

	slog.Info("loaded OAuth credentials from local file", "path", path)
	return parseCredentials(data)
}

func parseCredentials(data []byte) (*OAuthCredentials, error) {
	// Try standard Google credentials format first: {"web": {...}} or {"installed": {...}}
	var wrapper struct {
		Web struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"web"`
		Installed struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil {
		if wrapper.Web.ClientID != "" {
			return &OAuthCredentials{
				ClientID:     wrapper.Web.ClientID,
				ClientSecret: wrapper.Web.ClientSecret,
			}, nil
		}
		if wrapper.Installed.ClientID != "" {
			return &OAuthCredentials{
				ClientID:     wrapper.Installed.ClientID,
				ClientSecret: wrapper.Installed.ClientSecret,
			}, nil
		}
	}

	// Try flat format: {"client_id": ..., "client_secret": ...}
	var creds OAuthCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return nil, fmt.Errorf("credentials missing client_id or client_secret")
	}
	return &creds, nil
}

// validatePKCE validates a PKCE code verifier against a code challenge.
func validatePKCE(verifier, challenge, method string) bool {
	if method != "S256" {
		return false
	}

	hash := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(hash[:])
	return computed == challenge
}

// generateToken generates a cryptographically secure random token.
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random token: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// writeOAuthError writes a standard OAuth2 error response.
func writeOAuthError(w http.ResponseWriter, status int, errCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}
