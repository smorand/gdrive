package mcp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("body = %q, want %q", w.Body.String(), "OK")
	}
}

func TestAuthMiddleware(t *testing.T) {
	srv := &Server{
		config: &ServerConfig{
			BaseURL: "https://drive.mcp.example.com",
		},
		oauth2: newTestOAuth2Server(),
	}

	innerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := srv.authMiddleware(inner)

	t.Run("no auth header returns 401", func(t *testing.T) {
		innerCalled = false
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
		if innerCalled {
			t.Error("inner handler should not be called")
		}

		wwwAuth := w.Header().Get("WWW-Authenticate")
		if wwwAuth == "" {
			t.Error("expected WWW-Authenticate header")
		}
		if !contains(wwwAuth, "resource_metadata") {
			t.Errorf("WWW-Authenticate should contain resource_metadata, got: %s", wwwAuth)
		}
	})

	t.Run("invalid auth scheme returns 401", func(t *testing.T) {
		innerCalled = false
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("valid Bearer token passes through", func(t *testing.T) {
		innerCalled = false
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer valid-test-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if !innerCalled {
			t.Error("inner handler should be called")
		}
	})

	t.Run("empty Bearer token returns 401", func(t *testing.T) {
		innerCalled = false
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer ")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"error", "ERROR"},
		{"", "INFO"},
		{"unknown", "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLogLevel(tt.input)
			if level.String() != tt.want {
				t.Errorf("parseLogLevel(%q) = %s, want %s", tt.input, level.String(), tt.want)
			}
		})
	}
}

func TestSetupLogging(t *testing.T) {
	// Just verify it doesn't panic
	origEnv := os.Getenv("ENVIRONMENT")
	origLevel := os.Getenv("LOG_LEVEL")
	defer func() {
		os.Setenv("ENVIRONMENT", origEnv)
		os.Setenv("LOG_LEVEL", origLevel)
	}()

	os.Setenv("ENVIRONMENT", "prd")
	os.Setenv("LOG_LEVEL", "debug")
	setupLogging()

	os.Setenv("ENVIRONMENT", "dev")
	os.Setenv("LOG_LEVEL", "")
	setupLogging()
}

func TestNewServer(t *testing.T) {
	// Write a temporary credentials file
	tmpDir := t.TempDir()
	credFile := tmpDir + "/creds.json"
	os.WriteFile(credFile, []byte(`{"client_id":"test-id","client_secret":"test-secret"}`), 0600)

	cfg := &ServerConfig{
		Host:           "127.0.0.1",
		Port:           0,
		BaseURL:        "http://localhost:8080",
		CredentialFile: credFile,
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if srv.mcpServer == nil {
		t.Error("mcpServer should not be nil")
	}
	if srv.oauth2 == nil {
		t.Error("oauth2 should not be nil")
	}
}

func TestNewServerMissingCredentials(t *testing.T) {
	cfg := &ServerConfig{
		Host:    "127.0.0.1",
		Port:    0,
		BaseURL: "http://localhost:8080",
	}

	_, err := NewServer(cfg)
	if err == nil {
		t.Error("expected error for missing credentials")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
