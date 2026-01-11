// Package main is the entry point for the gdrive command.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gdrive/internal/cli"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gdrive",
		Short: "Google Drive sync tool",
		Long:  "A command-line tool for syncing files and folders with Google Drive",
	}

	// Setup global flags and pre-run hook
	cli.SetupRootCommand(rootCmd)

	// Add subcommands
	rootCmd.AddCommand(cli.FileCmd())
	rootCmd.AddCommand(cli.FolderCmd())
	rootCmd.AddCommand(cli.SearchCmd())
	rootCmd.AddCommand(cli.ActivityCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
