package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ThreatOptic/CLI/internal/config"
)

func newAuthCmd(opts *options) *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Manage ThreatOptic credentials",
	}
	auth.AddCommand(
		newAuthLoginCmd(opts),
		newAuthStatusCmd(opts),
		newAuthLogoutCmd(opts),
	)
	return auth
}

func newAuthLoginCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Store an API key after verifying it with the API",
		Long: "Store a ThreatOptic API key after verifying it with the API.\n\n" +
			"Create a key in the ThreatOptic dashboard, then run this command and paste it\n" +
			"at the prompt. The key is only written to disk once the API accepts it.\n\n" +
			"The key is taken from --api-key, then " + config.EnvAPIKey + ", and otherwise read\n" +
			"from the prompt. An already-stored key never suppresses the prompt.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			resolved := config.Resolve(opts.apiURL, opts.apiKey, file)

			apiKey, err := loginKey(opts, cmd)
			if err != nil {
				return err
			}
			if apiKey == "" {
				return errors.New("no API key provided")
			}

			// Verify before persisting so a bad key fails here rather than on
			// some unrelated command later.
			user, err := opts.newClient(resolved.APIURL, apiKey).WhoAmI(cmd.Context())
			if err != nil {
				return fmt.Errorf("could not verify API key against %s: %w", resolved.APIURL, err)
			}

			file.APIKey = apiKey
			file.APIURL = resolved.APIURL
			if err := config.Save(file); err != nil {
				return err
			}

			path, err := config.Path()
			if err != nil {
				return err
			}

			if opts.json {
				return opts.emitJSON(cmd, map[string]any{
					"id":          user.ID,
					"email":       user.Email,
					"api_url":     resolved.APIURL,
					"config_path": path,
				})
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s\nCredentials saved to %s\n", user.Email, path)
			return err
		},
	}
}

func newAuthStatusCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the configured endpoint and credential",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			apiClient, resolved, err := opts.authenticated()
			if err != nil {
				return err
			}

			user, err := apiClient.WhoAmI(cmd.Context())
			if err != nil {
				return withLoginHint(err)
			}

			masked := config.MaskKey(resolved.APIKey)

			if opts.json {
				return opts.emitJSON(cmd, map[string]any{
					"api_url":        resolved.APIURL,
					"api_url_source": string(resolved.APIURLSource),
					"api_key":        masked,
					"api_key_source": string(resolved.APIKeySource),
					"id":             user.ID,
					"email":          user.Email,
				})
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "API URL:  %s (%s)\n", resolved.APIURL, resolved.APIURLSource)
			fmt.Fprintf(out, "API key:  %s (%s)\n", masked, resolved.APIKeySource)
			_, err = fmt.Fprintf(out, "Account:  %s\n", user.Email)
			return err
		},
	}
}

func newAuthLogoutCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored API key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}

			removed := file.APIKey != ""
			if removed {
				file.APIKey = ""
				if err := config.Save(file); err != nil {
					return err
				}
			}

			if opts.json {
				return opts.emitJSON(cmd, map[string]any{"logged_out": removed})
			}

			message := "No stored API key.\n"
			if removed {
				message = "Logged out. Stored API key removed.\n"
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), message)
			return err
		},
	}
}

// loginKey sources the key to store. A key already on disk is deliberately
// skipped: someone running "auth login" is replacing it, not reusing it.
func loginKey(opts *options, cmd *cobra.Command) (string, error) {
	if flagKey := strings.TrimSpace(opts.apiKey); flagKey != "" {
		return flagKey, nil
	}
	if envKey := strings.TrimSpace(os.Getenv(config.EnvAPIKey)); envKey != "" {
		return envKey, nil
	}
	entered, err := opts.readKey(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(entered), nil
}

// promptAPIKey reads a key from the terminal without echoing it. The prompt
// goes to stderr so stdout stays free for command output.
func promptAPIKey(cmd *cobra.Command) (string, error) {
	if file, ok := cmd.InOrStdin().(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(cmd.ErrOrStderr(), "ThreatOptic API key: ")
		entered, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return "", fmt.Errorf("read API key: %w", err)
		}
		return string(entered), nil
	}

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read API key: %w", err)
	}
	return line, nil
}
