package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(opts *options, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.json {
				return opts.emitJSON(cmd, map[string]string{"version": version})
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version)
			return err
		},
	}
}
