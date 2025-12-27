package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags for configuration
	configDirFlag       string
	credentialsPathFlag string

	// Global config instance
	globalConfig *Config
)

var rootCmd = &cobra.Command{
	Use:   "gdrive",
	Short: "Google Drive sync tool",
	Long:  "A command-line tool for syncing files and folders with Google Drive",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize global config with priority: CLI flags > env vars > defaults
		globalConfig = NewConfig(configDirFlag, credentialsPathFlag)
	},
}

func init() {
	// Add persistent flags (available to all commands)
	rootCmd.PersistentFlags().StringVar(&configDirFlag, "config-dir", "",
		"Config directory (default: $HOME/.gdrive, env: GDRIVE_CONFIG_DIR)")
	rootCmd.PersistentFlags().StringVar(&credentialsPathFlag, "credentials", "",
		"Path to credentials.json file (env: GDRIVE_CREDENTIALS_PATH)")

	rootCmd.AddCommand(fileCmd)
	rootCmd.AddCommand(folderCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(activityCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
