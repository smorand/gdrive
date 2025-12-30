package main

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

const (
	// OAuth server configuration
	oauthServerPort     = ":8080"
	oauthCallbackPath   = "/oauth2callback"
	oauthRedirectURL    = "http://localhost:8080/oauth2callback"
	oauthServerTimeout  = 5 * time.Second
	oauthTimeout        = 3 * time.Minute
	oauthServerStartup  = 100 * time.Millisecond

	// File permissions
	configDirPerm       = 0755
	tokenFilePerm       = 0600

	// Default config paths
	defaultConfigDirName       = ".credentials"
	defaultTokenFileName       = "token_gdrive.json"
	defaultCredentialsFileName = "google_credentials.json"

	// Environment variable names
	envConfigDir        = "GDRIVE_CONFIG_DIR"
	envCredentialsPath  = "GDRIVE_CREDENTIALS_PATH"
)

// Config holds the configuration paths
type Config struct {
	ConfigDir       string
	CredentialsPath string
}

// NewConfig creates a new Config with priority: CLI args > env vars > defaults
func NewConfig(cliConfigDir, cliCredentialsPath string) *Config {
	cfg := &Config{}

	// Determine config directory: CLI > Env > Default
	if cliConfigDir != "" {
		cfg.ConfigDir = cliConfigDir
	} else if envDir := os.Getenv(envConfigDir); envDir != "" {
		cfg.ConfigDir = envDir
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			cfg.ConfigDir = defaultConfigDirName
		} else {
			cfg.ConfigDir = filepath.Join(home, defaultConfigDirName)
		}
	}

	// Determine credentials path: CLI > Env > Default lookup
	if cliCredentialsPath != "" {
		cfg.CredentialsPath = cliCredentialsPath
	} else if envCred := os.Getenv(envCredentialsPath); envCred != "" {
		cfg.CredentialsPath = envCred
	}
	// If still empty, will be resolved by GetCredentialsPath

	return cfg
}

// GetConfigDir returns the config directory path
func (c *Config) GetConfigDir() string {
	return c.ConfigDir
}

// GetTokenPath returns the token file path
func (c *Config) GetTokenPath() string {
	return filepath.Join(c.ConfigDir, defaultTokenFileName)
}

// GetCredentialsPath returns the credentials file path
func (c *Config) GetCredentialsPath() (string, error) {
	// If explicitly set via CLI or env, use it
	if c.CredentialsPath != "" {
		if _, err := os.Stat(c.CredentialsPath); err == nil {
			return c.CredentialsPath, nil
		}
		return "", fmt.Errorf("credentials file not found at %s", c.CredentialsPath)
	}

	// Try current directory first
	if _, err := os.Stat(defaultCredentialsFileName); err == nil {
		return defaultCredentialsFileName, nil
	}

	// Try config directory
	configPath := filepath.Join(c.ConfigDir, defaultCredentialsFileName)
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	return "", fmt.Errorf("%s not found in current directory or %s", defaultCredentialsFileName, c.ConfigDir)
}

// GetTokenFromWeb requests a token from the web using a local server
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

// SaveToken saves a token to a file path
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

// LoadToken retrieves a token from a local file
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

// GetAuthenticatedService returns an authenticated Drive service
func GetAuthenticatedService(cfg *Config) (*drive.Service, error) {
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

	client := config.Client(context.Background(), tok)
	srv, err := drive.New(client)
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive client: %v", err)
	}

	return srv, nil
}

// GetAuthenticatedActivityService returns an authenticated Drive Activity service
func GetAuthenticatedActivityService(cfg *Config) (*driveactivity.Service, error) {
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

	client := config.Client(context.Background(), tok)
	srv, err := driveactivity.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive Activity client: %v", err)
	}

	return srv, nil
}
