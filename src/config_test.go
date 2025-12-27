package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPriority(t *testing.T) {
	// Save original env vars
	origConfigDir := os.Getenv(envConfigDir)
	origCredPath := os.Getenv(envCredentialsPath)
	defer func() {
		os.Setenv(envConfigDir, origConfigDir)
		os.Setenv(envCredentialsPath, origCredPath)
	}()

	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, defaultConfigDirName)

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
			os.Setenv(envConfigDir, tt.envConfigDir)
			os.Setenv(envCredentialsPath, tt.envCredPath)

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
