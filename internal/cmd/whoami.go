package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ThreatOptic/CLI/internal/api"
)

func newWhoamiCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the account the current API key belongs to",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			apiClient, _, err := opts.authenticated()
			if err != nil {
				return err
			}

			user, err := apiClient.WhoAmI(cmd.Context())
			if err != nil {
				return withLoginHint(err)
			}

			if opts.json {
				return opts.emitJSON(cmd, map[string]any{
					"id":    user.ID,
					"email": user.Email,
					"roles": rolesOrEmpty(user),
				})
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s  (user %s, roles: %s)\n",
				user.Email, user.ID, formatRoles(user))
			return err
		},
	}
}

func rolesOrEmpty(user *api.User) []string {
	if user.Roles == nil {
		return []string{}
	}
	return user.Roles
}

func formatRoles(user *api.User) string {
	if len(user.Roles) == 0 {
		return "none"
	}
	return strings.Join(user.Roles, ", ")
}
