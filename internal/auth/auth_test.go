package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPriority(t *testing.T) {
	// Save original env vars
	origConfigDir := os.Getenv(EnvConfigDir)
	origCredPath := os.Getenv(EnvCredentialsPath)
	defer func() {
		os.Setenv(EnvConfigDir, origConfigDir)
		os.Setenv(EnvCredentialsPath, origCredPath)
	}()

	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, DefaultConfigDirName)

	tests := []struct {
		name            string
		cliConfigDir    string
		cliCredPath     string
		envConfigDir    string
		envCredPath     string
		expectedDir     string
		expectedCredSet bool
	}{
		{
			name:            "All defaults",
			cliConfigDir:    "",
			cliCredPath:     "",
			envConfigDir:    "",
			envCredPath:     "",
			expectedDir:     defaultDir,
			expectedCredSet: false,
		},
		{
			name:            "Env vars set",
			cliConfigDir:    "",
			cliCredPath:     "",
			envConfigDir:    "/env/path",
			envCredPath:     "/env/creds.json",
			expectedDir:     "/env/path",
			expectedCredSet: true,
		},
		{
			name:            "CLI overrides env",
			cliConfigDir:    "/cli/path",
			cliCredPath:     "/cli/creds.json",
			envConfigDir:    "/env/path",
			envCredPath:     "/env/creds.json",
			expectedDir:     "/cli/path",
			expectedCredSet: true,
		},
		{
			name:            "CLI partial override",
			cliConfigDir:    "/cli/path",
			cliCredPath:     "",
			envConfigDir:    "/env/path",
			envCredPath:     "/env/creds.json",
			expectedDir:     "/cli/path",
			expectedCredSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			os.Setenv(EnvConfigDir, tt.envConfigDir)
			os.Setenv(EnvCredentialsPath, tt.envCredPath)

			// Create config
			cfg := NewConfig(tt.cliConfigDir, tt.cliCredPath)

			// Check config dir
			if cfg.ConfigDir != tt.expectedDir {
				t.Errorf("ConfigDir = %v, want %v", cfg.ConfigDir, tt.expectedDir)
			}

			// Check credentials path
			if tt.expectedCredSet {
				expectedCred := tt.cliCredPath
				if expectedCred == "" {
					expectedCred = tt.envCredPath
				}
				if cfg.CredentialsPath != expectedCred {
					t.Errorf("CredentialsPath = %v, want %v", cfg.CredentialsPath, expectedCred)
				}
			} else {
				if cfg.CredentialsPath != "" {
					t.Errorf("CredentialsPath should be empty, got %v", cfg.CredentialsPath)
				}
			}
		})
	}
}

func TestConfigIntegration(t *testing.T) {
	// Save original env vars
	origConfigDir := os.Getenv(EnvConfigDir)
	origCredPath := os.Getenv(EnvCredentialsPath)
	defer func() {
		os.Setenv(EnvConfigDir, origConfigDir)
		os.Setenv(EnvCredentialsPath, origCredPath)
	}()

	t.Run("CLI flag takes precedence over env var", func(t *testing.T) {
		os.Setenv(EnvConfigDir, "/env/path")
		os.Setenv(EnvCredentialsPath, "/env/creds.json")

		cfg := NewConfig("/cli/path", "/cli/creds.json")

		if cfg.ConfigDir != "/cli/path" {
			t.Errorf("Expected ConfigDir=/cli/path, got %s", cfg.ConfigDir)
		}
		if cfg.CredentialsPath != "/cli/creds.json" {
			t.Errorf("Expected CredentialsPath=/cli/creds.json, got %s", cfg.CredentialsPath)
		}
	})

	t.Run("Env var takes precedence over default", func(t *testing.T) {
		os.Setenv(EnvConfigDir, "/env/custom")
		os.Setenv(EnvCredentialsPath, "/env/custom-creds.json")

		cfg := NewConfig("", "")

		if cfg.ConfigDir != "/env/custom" {
			t.Errorf("Expected ConfigDir=/env/custom, got %s", cfg.ConfigDir)
		}
		if cfg.CredentialsPath != "/env/custom-creds.json" {
			t.Errorf("Expected CredentialsPath=/env/custom-creds.json, got %s", cfg.CredentialsPath)
		}
	})

	t.Run("Default values when nothing specified", func(t *testing.T) {
		os.Unsetenv(EnvConfigDir)
		os.Unsetenv(EnvCredentialsPath)

		cfg := NewConfig("", "")

		home, _ := os.UserHomeDir()
		expectedDir := filepath.Join(home, DefaultConfigDirName)

		if cfg.ConfigDir != expectedDir {
			t.Errorf("Expected ConfigDir=%s, got %s", expectedDir, cfg.ConfigDir)
		}
		if cfg.CredentialsPath != "" {
			t.Errorf("Expected empty CredentialsPath, got %s", cfg.CredentialsPath)
		}
	})

	t.Run("GetTokenPath uses config dir", func(t *testing.T) {
		cfg := NewConfig("/custom/config", "")
		tokenPath := cfg.GetTokenPath()
		expected := filepath.Join("/custom/config", DefaultTokenFileName)

		if tokenPath != expected {
			t.Errorf("Expected token path=%s, got %s", expected, tokenPath)
		}
	})
}
