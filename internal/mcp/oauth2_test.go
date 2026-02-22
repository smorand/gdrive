package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestOAuth2Server() *OAuth2Server {
	creds := &OAuthCredentials{
		ClientID:     "test-google-client-id",
		ClientSecret: "test-google-client-secret",
	}
	return NewOAuth2Server("https://drive.mcp.example.com", creds)
}

func TestHandleProtectedResourceMetadata(t *testing.T) {
	srv := newTestOAuth2Server()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	w := httptest.NewRecorder()

	srv.HandleProtectedResourceMetadata(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if meta["resource"] != "https://drive.mcp.example.com" {
		t.Errorf("resource = %q, want %q", meta["resource"], "https://drive.mcp.example.com")
	}

	servers, ok := meta["authorization_servers"].([]interface{})
	if !ok || len(servers) == 0 {
		t.Fatal("expected authorization_servers to be non-empty array")
	}
	if servers[0] != "https://drive.mcp.example.com" {
		t.Errorf("authorization_servers[0] = %q, want %q", servers[0], "https://drive.mcp.example.com")
	}
}

func TestHandleAuthorizationServerMetadata(t *testing.T) {
	srv := newTestOAuth2Server()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	srv.HandleAuthorizationServerMetadata(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if meta["issuer"] != "https://drive.mcp.example.com" {
		t.Errorf("issuer = %q, want %q", meta["issuer"], "https://drive.mcp.example.com")
	}
	if meta["authorization_endpoint"] != "https://drive.mcp.example.com/oauth/authorize" {
		t.Errorf("authorization_endpoint = %q", meta["authorization_endpoint"])
	}
	if meta["token_endpoint"] != "https://drive.mcp.example.com/oauth/token" {
		t.Errorf("token_endpoint = %q", meta["token_endpoint"])
	}
	if meta["registration_endpoint"] != "https://drive.mcp.example.com/oauth/register" {
		t.Errorf("registration_endpoint = %q", meta["registration_endpoint"])
	}

	methods, ok := meta["code_challenge_methods_supported"].([]interface{})
	if !ok || len(methods) == 0 || methods[0] != "S256" {
		t.Errorf("expected code_challenge_methods_supported to include S256")
	}
}

func TestHandleClientRegistration(t *testing.T) {
	srv := newTestOAuth2Server()

	t.Run("successful registration", func(t *testing.T) {
		body := `{"redirect_uris": ["http://localhost:3000/callback"]}`
		req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.HandleClientRegistration(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp["client_id"] == "" {
			t.Error("expected non-empty client_id")
		}
		if resp["client_secret"] == "" {
			t.Error("expected non-empty client_secret")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/register", nil)
		w := httptest.NewRecorder()

		srv.HandleClientRegistration(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", w.Code)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader("{bad}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.HandleClientRegistration(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleAuthorize(t *testing.T) {
	srv := newTestOAuth2Server()

	t.Run("missing client_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?redirect_uri=http://localhost", nil)
		w := httptest.NewRecorder()

		srv.HandleAuthorize(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing redirect_uri", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=test", nil)
		w := httptest.NewRecorder()

		srv.HandleAuthorize(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("redirects to Google", func(t *testing.T) {
		params := url.Values{
			"client_id":             {"test-client"},
			"redirect_uri":          {"http://localhost:3000/callback"},
			"code_challenge":        {"challenge123"},
			"code_challenge_method": {"S256"},
			"state":                 {"client-state-abc"},
		}
		req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		srv.HandleAuthorize(w, req)

		if w.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", w.Code)
		}

		location := w.Header().Get("Location")
		if !strings.Contains(location, "accounts.google.com") {
			t.Errorf("expected redirect to Google, got: %s", location)
		}

		// Verify client was auto-registered
		srv.mu.RLock()
		_, exists := srv.clients["test-client"]
		srv.mu.RUnlock()
		if !exists {
			t.Error("expected client to be auto-registered")
		}
	})
}

func TestHandleToken(t *testing.T) {
	srv := newTestOAuth2Server()

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/token", nil)
		w := httptest.NewRecorder()

		srv.HandleToken(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", w.Code)
		}
	})

	t.Run("unsupported grant type", func(t *testing.T) {
		form := url.Values{"grant_type": {"implicit"}}
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.HandleToken(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}

		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["error"] != "unsupported_grant_type" {
			t.Errorf("error = %q, want unsupported_grant_type", resp["error"])
		}
	})

	t.Run("invalid authorization code", func(t *testing.T) {
		form := url.Values{
			"grant_type": {"authorization_code"},
			"code":       {"invalid-code"},
		}
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.HandleToken(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}

		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["error"] != "invalid_grant" {
			t.Errorf("error = %q, want invalid_grant", resp["error"])
		}
	})

	t.Run("missing code", func(t *testing.T) {
		form := url.Values{"grant_type": {"authorization_code"}}
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.HandleToken(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing refresh_token", func(t *testing.T) {
		form := url.Values{"grant_type": {"refresh_token"}}
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.HandleToken(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestValidatePKCE(t *testing.T) {
	tests := []struct {
		name      string
		verifier  string
		challenge string
		method    string
		want      bool
	}{
		{
			name:      "valid S256",
			verifier:  "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			want:      true,
		},
		{
			name:      "invalid verifier",
			verifier:  "wrong-verifier",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			want:      false,
		},
		{
			name:      "unsupported method",
			verifier:  "test",
			challenge: "test",
			method:    "plain",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validatePKCE(tt.verifier, tt.challenge, tt.method)
			if got != tt.want {
				t.Errorf("validatePKCE() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateAccessToken(t *testing.T) {
	srv := newTestOAuth2Server()

	t.Run("valid token", func(t *testing.T) {
		config, token, err := srv.ValidateAccessToken("test-access-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.ClientID != "test-google-client-id" {
			t.Errorf("ClientID = %q, want %q", config.ClientID, "test-google-client-id")
		}
		if token.AccessToken != "test-access-token" {
			t.Errorf("AccessToken = %q, want %q", token.AccessToken, "test-access-token")
		}
	})

	t.Run("empty token", func(t *testing.T) {
		_, _, err := srv.ValidateAccessToken("")
		if err == nil {
			t.Error("expected error for empty token")
		}
	})
}

func TestParseCredentials(t *testing.T) {
	t.Run("web format", func(t *testing.T) {
		data := []byte(`{"web":{"client_id":"web-id","client_secret":"web-secret"}}`)
		creds, err := parseCredentials(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds.ClientID != "web-id" {
			t.Errorf("ClientID = %q, want %q", creds.ClientID, "web-id")
		}
		if creds.ClientSecret != "web-secret" {
			t.Errorf("ClientSecret = %q, want %q", creds.ClientSecret, "web-secret")
		}
	})

	t.Run("installed format", func(t *testing.T) {
		data := []byte(`{"installed":{"client_id":"inst-id","client_secret":"inst-secret"}}`)
		creds, err := parseCredentials(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds.ClientID != "inst-id" {
			t.Errorf("ClientID = %q, want %q", creds.ClientID, "inst-id")
		}
	})

	t.Run("flat format", func(t *testing.T) {
		data := []byte(`{"client_id":"flat-id","client_secret":"flat-secret"}`)
		creds, err := parseCredentials(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds.ClientID != "flat-id" {
			t.Errorf("ClientID = %q, want %q", creds.ClientID, "flat-id")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		data := []byte(`{"client_id":"only-id"}`)
		_, err := parseCredentials(data)
		if err == nil {
			t.Error("expected error for missing client_secret")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		data := []byte(`{bad json}`)
		_, err := parseCredentials(data)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestGenerateToken(t *testing.T) {
	token1 := generateToken()
	token2 := generateToken()

	if len(token1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length = %d, want 64", len(token1))
	}
	if token1 == token2 {
		t.Error("expected unique tokens")
	}
}

func TestWriteOAuthError(t *testing.T) {
	w := httptest.NewRecorder()
	writeOAuthError(w, http.StatusBadRequest, "invalid_request", "test description")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "invalid_request" {
		t.Errorf("error = %q, want invalid_request", resp["error"])
	}
	if resp["error_description"] != "test description" {
		t.Errorf("error_description = %q, want %q", resp["error_description"], "test description")
	}
}

func TestHandleCallbackError(t *testing.T) {
	srv := newTestOAuth2Server()

	t.Run("Google error parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/callback?error=access_denied", nil)
		w := httptest.NewRecorder()

		srv.HandleCallback(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing code", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=abc", nil)
		w := httptest.NewRecorder()

		srv.HandleCallback(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=abc&state=invalid", nil)
		w := httptest.NewRecorder()

		srv.HandleCallback(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}
