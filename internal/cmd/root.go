// Package cmd wires the ThreatOptic CLI command tree.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ThreatOptic/CLI/internal/api"
	"github.com/ThreatOptic/CLI/internal/config"
)

// client is the API surface the commands depend on, kept as an interface so
// tests can substitute a fake.
type client interface {
	WhoAmI(ctx context.Context) (*api.User, error)
	Check(ctx context.Context, query string) (*api.CheckResult, error)
}

// errNoAPIKey is returned before any request is attempted.
var errNoAPIKey = errors.New("no API key configured. Run 'threatoptic auth login'")

type options struct {
	apiURL string
	apiKey string
	json   bool

	newClient func(baseURL, apiKey string) client
	readKey   func(cmd *cobra.Command) (string, error)
}

// NewRoot builds the root command.
func NewRoot(version string) *cobra.Command {
	return newRoot(version, &options{
		newClient: func(baseURL, apiKey string) client { return api.New(baseURL, apiKey) },
		readKey:   promptAPIKey,
	})
}

func newRoot(version string, opts *options) *cobra.Command {
	root := &cobra.Command{
		Use:   "threatoptic",
		Short: "ThreatOptic command-line client",
		Long:  "Command-line client for the ThreatOptic security monitoring platform.",
		// Usage text on a runtime failure buries the actual error.
		SilenceUsage: true,
	}

	flags := root.PersistentFlags()
	flags.StringVar(&opts.apiURL, "api-url", "", "ThreatOptic API base URL (default "+config.DefaultAPIURL+")")
	flags.StringVar(&opts.apiKey, "api-key", "", "ThreatOptic API key (prefer 'threatoptic auth login')")
	flags.BoolVar(&opts.json, "json", false, "output JSON instead of human-readable text")

	root.AddCommand(
		newVersionCmd(opts, version),
		newWhoamiCmd(opts),
		newAuthCmd(opts),
		newCheckCmd(opts),
	)
	return root
}

// resolve merges flags, environment, and the config file.
func (o *options) resolve() (config.Resolved, error) {
	file, err := config.Load()
	if err != nil {
		return config.Resolved{}, err
	}
	return config.Resolve(o.apiURL, o.apiKey, file), nil
}

// authenticated resolves configuration and returns a ready client, failing
// before any network call when no credential is available.
func (o *options) authenticated() (client, config.Resolved, error) {
	resolved, err := o.resolve()
	if err != nil {
		return nil, config.Resolved{}, err
	}
	if resolved.APIKey == "" {
		return nil, resolved, errNoAPIKey
	}
	return o.newClient(resolved.APIURL, resolved.APIKey), resolved, nil
}

func (o *options) emitJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

// withLoginHint turns a bare 401 into an actionable message.
func withLoginHint(err error) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Unauthorized() {
		return fmt.Errorf("%s (run 'threatoptic auth login' to re-authenticate)", apiErr.Error())
	}
	return err
}
