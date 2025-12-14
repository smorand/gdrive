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

	// Config paths
	configDirName       = ".gdrive"
	tokenFileName       = "token.json"
	credentialsFileName = "credentials.json"
)

// GetConfigDir returns the config directory path
func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return configDirName
	}
	return filepath.Join(home, configDirName)
}

// GetTokenPath returns the token file path
func GetTokenPath() string {
	return filepath.Join(GetConfigDir(), tokenFileName)
}

// GetCredentialsPath returns the credentials file path
func GetCredentialsPath() (string, error) {
	// Try current directory first
	if _, err := os.Stat(credentialsFileName); err == nil {
		return credentialsFileName, nil
	}

	// Try config directory
	configPath := filepath.Join(GetConfigDir(), credentialsFileName)
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	return "", fmt.Errorf("%s not found in current directory or %s", credentialsFileName, GetConfigDir())
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
func GetAuthenticatedService() (*drive.Service, error) {
	credPath, err := GetCredentialsPath()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials file: %v", err)
	}

	tokenPath := GetTokenPath()
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
