package cli

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed skill.md
var skillContent string

// SkillCmd returns the skill command. It prints the embedded skill markdown
// (the canonical AI-agent guide for the gdrive CLI and MCP server) to stdout.
func SkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the gdrive skill (AI agent guide) to stdout",
		Long: `Print the gdrive skill — the canonical guide for AI agents on how to
use the gdrive CLI and MCP server. The output is markdown with YAML
frontmatter (name, description) and is the single source of truth for
AI consumers; it is regenerated from the binary, so it always matches
the installed version.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprint(cmd.OutOrStdout(), skillContent); err != nil {
				return fmt.Errorf("write skill: %w", err)
			}
			return nil
		},
	}
}
