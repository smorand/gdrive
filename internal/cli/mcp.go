package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"gdrive/internal/mcp"
)

// MCPCmd returns the MCP server command.
func MCPCmd() *cobra.Command {
	var (
		port           int
		host           string
		baseURL        string
		secretName     string
		secretProject  string
		credentialFile string
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP HTTP Streamable server",
		Long:  "Start an MCP (Model Context Protocol) HTTP Streamable server that exposes Google Drive operations as MCP tools for AI agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			if baseURL == "" {
				baseURL = fmt.Sprintf("http://localhost:%d", port)
			}

			cfg := &mcp.ServerConfig{
				Host:           host,
				Port:           port,
				BaseURL:        baseURL,
				SecretName:     secretName,
				SecretProject:  secretProject,
				CredentialFile: credentialFile,
			}

			srv, err := mcp.NewServer(cfg)
			if err != nil {
				return fmt.Errorf("create MCP server: %w", err)
			}

			return srv.Start()
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "Server port")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "Server host")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "External base URL (default: http://localhost:{port})")
	cmd.Flags().StringVar(&secretName, "secret-name", "", "GCP Secret Manager secret name for OAuth credentials")
	cmd.Flags().StringVar(&secretProject, "secret-project", "", "GCP project ID for Secret Manager")
	cmd.Flags().StringVar(&credentialFile, "credential-file", "", "Path to local OAuth credentials file (fallback)")

	return cmd
}
