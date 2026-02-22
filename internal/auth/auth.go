// Package auth provides OAuth2 authentication for Google Drive API.
// Supports two modes:
//   - CLI mode: credentials and tokens from local files
//   - MCP mode: OAuth config and access token injected via context
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/option"
)

// Context keys for MCP mode token injection.
type contextKey string

const (
	ctxKeyOAuthConfig contextKey = "oauth_config"
	ctxKeyAccessToken contextKey = "access_token"
)

const (
	// OAuth server configuration
	oauthServerPort    = ":8080"
	oauthCallbackPath  = "/oauth2callback"
	oauthRedirectURL   = "http://localhost:8080/oauth2callback"
	oauthServerTimeout = 5 * time.Second
	oauthTimeout       = 3 * time.Minute
	oauthServerStartup = 100 * time.Millisecond

	// File permissions
	configDirPerm = 0755
	tokenFilePerm = 0600

	// Default config paths
	DefaultConfigDirName       = ".credentials"
	DefaultTokenFileName       = "token_gdrive.json"
	DefaultCredentialsFileName = "google_credentials.json"

	// Environment variable names
	EnvConfigDir       = "GDRIVE_CONFIG_DIR"
	EnvCredentialsPath = "GDRIVE_CREDENTIALS_PATH"
)

// Config holds the configuration paths for authentication.
type Config struct {
	ConfigDir       string
	CredentialsPath string
}

// NewConfig creates a new Config with priority: CLI args > env vars > defaults.
func NewConfig(cliConfigDir, cliCredentialsPath string) *Config {
	cfg := &Config{}

	// Determine config directory: CLI > Env > Default
	if cliConfigDir != "" {
		cfg.ConfigDir = cliConfigDir
	} else if envDir := os.Getenv(EnvConfigDir); envDir != "" {
		cfg.ConfigDir = envDir
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			cfg.ConfigDir = DefaultConfigDirName
		} else {
			cfg.ConfigDir = filepath.Join(home, DefaultConfigDirName)
		}
	}

	// Determine credentials path: CLI > Env > Default lookup
	if cliCredentialsPath != "" {
		cfg.CredentialsPath = cliCredentialsPath
	} else if envCred := os.Getenv(EnvCredentialsPath); envCred != "" {
		cfg.CredentialsPath = envCred
	}
	// If still empty, will be resolved by GetCredentialsPath

	return cfg
}

// GetConfigDir returns the config directory path.
func (c *Config) GetConfigDir() string {
	return c.ConfigDir
}

// GetTokenPath returns the token file path.
func (c *Config) GetTokenPath() string {
	return filepath.Join(c.ConfigDir, DefaultTokenFileName)
}

// GetCredentialsPath returns the credentials file path.
func (c *Config) GetCredentialsPath() (string, error) {
	// If explicitly set via CLI or env, use it
	if c.CredentialsPath != "" {
		if _, err := os.Stat(c.CredentialsPath); err == nil {
			return c.CredentialsPath, nil
		}
		return "", fmt.Errorf("credentials file not found at %s", c.CredentialsPath)
	}

	// Try current directory first
	if _, err := os.Stat(DefaultCredentialsFileName); err == nil {
		return DefaultCredentialsFileName, nil
	}

	// Try config directory
	configPath := filepath.Join(c.ConfigDir, DefaultCredentialsFileName)
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	return "", fmt.Errorf("%s not found in current directory or %s", DefaultCredentialsFileName, c.ConfigDir)
}

// GetTokenFromWeb requests a token from the web using a local server.
func GetTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	// Use localhost with configured port
	config.RedirectURL = oauthRedirectURL

	// Create channels for communication
	codeChan := make(chan string)
	errChan := make(chan error)

	// Start local HTTP server
	server := &http.Server{Addr: oauthServerPort}
	http.HandleFunc(oauthCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			return
		}

		// Send success message to browser
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<html>
			<body>
				<h1>Authentication successful!</h1>
				<p>You can close this window and return to the terminal.</p>
			</body>
			</html>
		`)

		codeChan <- code
	})

	// Start server in background
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Ignore server closed error
			if err != http.ErrServerClosed {
				errChan <- err
			}
		}
	}()

	// Wait a moment for server to start
	time.Sleep(oauthServerStartup)

	// Generate auth URL
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Opening browser for authentication...\n")
	fmt.Printf("If browser doesn't open, visit:\n%v\n\n", authURL)

	// Try to open browser automatically
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", authURL)
	case "linux":
		cmd = exec.Command("xdg-open", authURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", authURL)
	}

	if cmd != nil {
		_ = cmd.Start()
	}

	// Wait for auth code or error
	var code string
	select {
	case code = <-codeChan:
		// Success
	case err := <-errChan:
		return nil, err
	case <-time.After(oauthTimeout):
		return nil, fmt.Errorf("authentication timeout after %v", oauthTimeout)
	}

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), oauthServerTimeout)
	defer cancel()
	_ = server.Shutdown(ctx)

	// Exchange code for token
	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %v", err)
	}

	fmt.Println("\nAuthentication successful!")
	return tok, nil
}

// SaveToken saves a token to a file path.
func SaveToken(path string, token *oauth2.Token) error {
	// Create config directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, configDirPerm); err != nil {
		return fmt.Errorf("unable to create config directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, tokenFilePerm)
	if err != nil {
		return fmt.Errorf("unable to cache oauth token: %w", err)
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

// LoadToken retrieves a token from a local file.
func LoadToken(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// WithOAuthConfig injects an OAuth2 config into the context (MCP mode).
func WithOAuthConfig(ctx context.Context, config *oauth2.Config) context.Context {
	return context.WithValue(ctx, ctxKeyOAuthConfig, config)
}

// GetOAuthConfigFromContext retrieves the OAuth2 config from context.
func GetOAuthConfigFromContext(ctx context.Context) (*oauth2.Config, bool) {
	config, ok := ctx.Value(ctxKeyOAuthConfig).(*oauth2.Config)
	return config, ok
}

// WithAccessToken injects an OAuth2 token into the context (MCP mode).
func WithAccessToken(ctx context.Context, token *oauth2.Token) context.Context {
	return context.WithValue(ctx, ctxKeyAccessToken, token)
}

// GetAccessTokenFromContext retrieves the OAuth2 token from context.
func GetAccessTokenFromContext(ctx context.Context) (*oauth2.Token, bool) {
	token, ok := ctx.Value(ctxKeyAccessToken).(*oauth2.Token)
	return token, ok
}

// GetClientFromContext creates an HTTP client from context-injected credentials.
// Returns nil if no credentials are in the context.
func GetClientFromContext(ctx context.Context) *http.Client {
	config, hasConfig := GetOAuthConfigFromContext(ctx)
	token, hasToken := GetAccessTokenFromContext(ctx)
	if hasConfig && hasToken {
		return config.Client(ctx, token)
	}
	return nil
}

// GetAuthenticatedService returns an authenticated Drive service.
// In MCP mode (context has OAuth config + token), uses context credentials.
// In CLI mode, uses file-based credentials.
func GetAuthenticatedService(cfg *Config) (*drive.Service, error) {
	return GetAuthenticatedServiceWithContext(context.Background(), cfg)
}

// GetAuthenticatedServiceWithContext returns an authenticated Drive service using context.
func GetAuthenticatedServiceWithContext(ctx context.Context, cfg *Config) (*drive.Service, error) {
	// MCP mode: check context for injected credentials
	if client := GetClientFromContext(ctx); client != nil {
		srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("unable to create Drive client: %v", err)
		}
		return srv, nil
	}

	// CLI mode: file-based credentials
	credPath, err := cfg.GetCredentialsPath()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope, driveactivity.DriveActivityReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials file: %v", err)
	}

	tokenPath := cfg.GetTokenPath()
	tok, err := LoadToken(tokenPath)
	if err != nil {
		// Get new token
		tok, err = GetTokenFromWeb(config)
		if err != nil {
			return nil, err
		}
		if err := SaveToken(tokenPath, tok); err != nil {
			return nil, err
		}
	}

	client := config.Client(ctx, tok)
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive client: %v", err)
	}

	return srv, nil
}

// GetAuthenticatedActivityService returns an authenticated Drive Activity service.
func GetAuthenticatedActivityService(cfg *Config) (*driveactivity.Service, error) {
	return GetAuthenticatedActivityServiceWithContext(context.Background(), cfg)
}

// GetAuthenticatedActivityServiceWithContext returns an authenticated Drive Activity service using context.
func GetAuthenticatedActivityServiceWithContext(ctx context.Context, cfg *Config) (*driveactivity.Service, error) {
	// MCP mode: check context for injected credentials
	if client := GetClientFromContext(ctx); client != nil {
		srv, err := driveactivity.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("unable to create Drive Activity client: %v", err)
		}
		return srv, nil
	}

	// CLI mode: file-based credentials
	credPath, err := cfg.GetCredentialsPath()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope, driveactivity.DriveActivityReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials file: %v", err)
	}

	tokenPath := cfg.GetTokenPath()
	tok, err := LoadToken(tokenPath)
	if err != nil {
		// Get new token
		tok, err = GetTokenFromWeb(config)
		if err != nil {
			return nil, err
		}
		if err := SaveToken(tokenPath, tok); err != nil {
			return nil, err
		}
	}

	client := config.Client(ctx, tok)
	srv, err := driveactivity.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive Activity client: %v", err)
	}

	return srv, nil
}
