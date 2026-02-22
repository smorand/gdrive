package cli

import (
	"fmt"
	"os"
	"strconv"

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
			// Environment variable fallbacks (for Cloud Run deployment)
			if !cmd.Flags().Changed("port") {
				if v := os.Getenv("PORT"); v != "" {
					if p, err := strconv.Atoi(v); err == nil {
						port = p
					}
				}
			}
			if !cmd.Flags().Changed("host") {
				if v := os.Getenv("HOST"); v != "" {
					host = v
				}
			}
			if !cmd.Flags().Changed("base-url") {
				if v := os.Getenv("BASE_URL"); v != "" {
					baseURL = v
				}
			}
			if !cmd.Flags().Changed("secret-name") {
				if v := os.Getenv("SECRET_NAME"); v != "" {
					secretName = v
				}
			}
			if !cmd.Flags().Changed("secret-project") {
				if v := os.Getenv("SECRET_PROJECT"); v != "" {
					secretProject = v
				}
			}
			if !cmd.Flags().Changed("credential-file") {
				if v := os.Getenv("CREDENTIAL_FILE"); v != "" {
					credentialFile = v
				}
			}

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

	cmd.Flags().IntVar(&port, "port", 8080, "Server port (env: PORT)")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "Server host (env: HOST)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "External base URL (env: BASE_URL)")
	cmd.Flags().StringVar(&secretName, "secret-name", "", "GCP Secret Manager secret name (env: SECRET_NAME)")
	cmd.Flags().StringVar(&secretProject, "secret-project", "", "GCP project ID for Secret Manager (env: SECRET_PROJECT)")
	cmd.Flags().StringVar(&credentialFile, "credential-file", "", "Path to local OAuth credentials file (env: CREDENTIAL_FILE)")

	return cmd
}
