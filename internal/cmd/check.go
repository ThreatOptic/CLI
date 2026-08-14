package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ThreatOptic/CLI/internal/api"
)

func newCheckCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "check <query>",
		Short: "Look up live lookalikes for a domain, package, or MCP name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, _, err := opts.authenticated()
			if err != nil {
				return err
			}

			result, err := apiClient.Check(cmd.Context(), args[0])
			if err != nil {
				return withLoginHint(err)
			}

			if opts.json {
				return opts.emitJSON(cmd, result)
			}

			return writeCheckHuman(cmd, result)
		},
	}
}

func writeCheckHuman(cmd *cobra.Command, result *api.CheckResult) error {
	mcp := "false"
	if result.MCP {
		mcp = "true"
	}
	present := "false"
	if result.SubjectPresent {
		present = "true"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(),
		"kind: %s\nsubject: %s\npresent: %s\nmcp: %s\ngenerated: %d\n",
		result.Kind, result.Subject, present, mcp, result.GeneratedCount,
	); err != nil {
		return err
	}
	for _, item := range result.Lookalikes {
		seen := "-"
		if item.FirstSeenAt != nil && *item.FirstSeenAt != "" {
			seen = *item.FirstSeenAt
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", item.Name, item.Technique, seen); err != nil {
			return err
		}
	}
	return nil
}
