// Package main is the entry point for the gdrive command.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"gdrive/internal/cli"
	"gdrive/internal/telemetry"
)

func main() {
	os.Exit(run())
}

func run() int {
	shutdown, err := telemetry.InitFromEnv("gdrive")
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: %v\n", err)
		return 1
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "telemetry shutdown: %v\n", err)
		}
	}()

	rootCmd := &cobra.Command{
		Use:   "gdrive",
		Short: "Google Drive sync tool",
		Long:  "A command-line tool for syncing files and folders with Google Drive",
	}

	cli.SetupRootCommand(rootCmd)

	rootCmd.AddCommand(cli.FileCmd())
	rootCmd.AddCommand(cli.FolderCmd())
	rootCmd.AddCommand(cli.SearchCmd())
	rootCmd.AddCommand(cli.ActivityCmd())
	rootCmd.AddCommand(cli.MCPCmd())
	rootCmd.AddCommand(cli.SkillCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
