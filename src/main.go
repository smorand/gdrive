package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gdrive",
	Short: "Google Drive sync tool",
	Long:  "A command-line tool for syncing files and folders with Google Drive",
}

func init() {
	rootCmd.AddCommand(fileCmd)
	rootCmd.AddCommand(folderCmd)
	rootCmd.AddCommand(searchCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
