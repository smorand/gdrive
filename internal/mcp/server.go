package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gdrive/internal/auth"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ServerConfig holds the MCP server configuration.
type ServerConfig struct {
	Host           string
	Port           int
	BaseURL        string
	SecretName     string
	SecretProject  string
	CredentialFile string
}

// Server is the MCP HTTP Streamable server for Google Drive.
type Server struct {
	config     *ServerConfig
	mcpServer  *server.MCPServer
	oauth2     *OAuth2Server
	httpServer *http.Server
}

// NewServer creates and configures the MCP server.
func NewServer(cfg *ServerConfig) (*Server, error) {
	setupLogging()

	// Load OAuth credentials
	creds, err := LoadOAuthCredentials(cfg.SecretName, cfg.SecretProject, cfg.CredentialFile)
	if err != nil {
		return nil, fmt.Errorf("load OAuth credentials: %w", err)
	}

	// Create OAuth2 server
	oauth2Srv := NewOAuth2Server(cfg.BaseURL, creds)

	// Create MCP server
	mcpSrv := server.NewMCPServer(
		"gdrive-mcp-server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s := &Server{
		config:    cfg,
		mcpServer: mcpSrv,
		oauth2:    oauth2Srv,
	}

	// Register ping tool
	s.registerPingTool()

	return s, nil
}

// GetMCPServer returns the underlying MCP server for tool registration.
func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}

// GetOAuth2Server returns the OAuth2 server for token validation.
func (s *Server) GetOAuth2Server() *OAuth2Server {
	return s.oauth2
}

// Start starts the MCP server with graceful shutdown.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	// Create the StreamableHTTP server with auth context injection
	streamableServer := server.NewStreamableHTTPServer(s.mcpServer,
		server.WithEndpointPath("/mcp"),
		server.WithHTTPContextFunc(s.httpContextFunc),
	)

	// Build HTTP mux
	mux := http.NewServeMux()

	// Health endpoint (no auth)
	mux.HandleFunc("GET /health", handleHealth)

	// OAuth endpoints (no auth)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.oauth2.HandleProtectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.oauth2.HandleAuthorizationServerMetadata)
	mux.HandleFunc("POST /oauth/register", s.oauth2.HandleClientRegistration)
	mux.HandleFunc("GET /oauth/authorize", s.oauth2.HandleAuthorize)
	mux.HandleFunc("GET /oauth/callback", s.oauth2.HandleCallback)
	mux.HandleFunc("POST /oauth/token", s.oauth2.HandleToken)

	// MCP endpoint (auth enforced in httpContextFunc via WithHTTPContextFunc)
	mux.Handle("/mcp", s.authMiddleware(streamableServer))

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("shutdown signal received, shutting down gracefully")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("starting MCP server", "addr", addr, "base_url", s.config.BaseURL)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

// httpContextFunc injects auth context from the HTTP request into the MCP context.
// This is called by the mcp-go SDK for each request to the /mcp endpoint.
func (s *Server) httpContextFunc(ctx context.Context, r *http.Request) context.Context {
	// Extract bearer token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ctx
	}

	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate and get OAuth config + token
	oauthConfig, token, err := s.oauth2.ValidateAccessToken(accessToken)
	if err != nil {
		slog.Warn("invalid access token", "error", err)
		return ctx
	}

	// Inject into context using the auth package functions
	ctx = auth.WithOAuthConfig(ctx, oauthConfig)
	ctx = auth.WithAccessToken(ctx, token)

	return ctx
}

// authMiddleware wraps an HTTP handler to enforce Bearer token authentication.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			resourceMetadata := fmt.Sprintf("%s/.well-known/oauth-protected-resource", s.config.BaseURL)
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadata))
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		accessToken := strings.TrimPrefix(authHeader, "Bearer ")
		if _, _, err := s.oauth2.ValidateAccessToken(accessToken); err != nil {
			resourceMetadata := fmt.Sprintf("%s/.well-known/oauth-protected-resource", s.config.BaseURL)
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_token", resource_metadata="%s"`, resourceMetadata))
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// registerPingTool registers the ping tool for connectivity testing.
func (s *Server) registerPingTool() {
	tool := mcp.NewTool("ping",
		mcp.WithDescription("Test MCP connectivity. Returns pong with current server time."),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		result := mcp.NewToolResultText(fmt.Sprintf(`{"message":"pong","time":"%s"}`, time.Now().Format(time.RFC3339)))
		slog.Info("tool call", "tool", "ping", "duration", time.Since(start))
		return result, nil
	})
}

// handleHealth handles the /health endpoint.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// setupLogging configures slog based on environment.
func setupLogging() {
	var handler slog.Handler

	if os.Getenv("ENVIRONMENT") == "prd" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
		})
	}

	slog.SetDefault(slog.New(handler))
}

// parseLogLevel parses a log level string.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
