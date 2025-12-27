package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigIntegration(t *testing.T) {
	// Save original env vars
	origConfigDir := os.Getenv(envConfigDir)
	origCredPath := os.Getenv(envCredentialsPath)
	defer func() {
		os.Setenv(envConfigDir, origConfigDir)
		os.Setenv(envCredentialsPath, origCredPath)
	}()

	t.Run("CLI flag takes precedence over env var", func(t *testing.T) {
		os.Setenv(envConfigDir, "/env/path")
		os.Setenv(envCredentialsPath, "/env/creds.json")

		cfg := NewConfig("/cli/path", "/cli/creds.json")

		if cfg.ConfigDir != "/cli/path" {
			t.Errorf("Expected ConfigDir=/cli/path, got %s", cfg.ConfigDir)
		}
		if cfg.CredentialsPath != "/cli/creds.json" {
			t.Errorf("Expected CredentialsPath=/cli/creds.json, got %s", cfg.CredentialsPath)
		}
	})

	t.Run("Env var takes precedence over default", func(t *testing.T) {
		os.Setenv(envConfigDir, "/env/custom")
		os.Setenv(envCredentialsPath, "/env/custom-creds.json")

		cfg := NewConfig("", "")

		if cfg.ConfigDir != "/env/custom" {
			t.Errorf("Expected ConfigDir=/env/custom, got %s", cfg.ConfigDir)
		}
		if cfg.CredentialsPath != "/env/custom-creds.json" {
			t.Errorf("Expected CredentialsPath=/env/custom-creds.json, got %s", cfg.CredentialsPath)
		}
	})

	t.Run("Default values when nothing specified", func(t *testing.T) {
		os.Unsetenv(envConfigDir)
		os.Unsetenv(envCredentialsPath)

		cfg := NewConfig("", "")

		home, _ := os.UserHomeDir()
		expectedDir := filepath.Join(home, defaultConfigDirName)

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
		expected := filepath.Join("/custom/config", defaultTokenFileName)

		if tokenPath != expected {
			t.Errorf("Expected token path=%s, got %s", expected, tokenPath)
		}
	})
}
