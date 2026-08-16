// Package config loads and stores the ThreatOptic CLI configuration and
// resolves effective settings from flags, environment, and file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultAPIURL is used when no URL is configured.
	DefaultAPIURL = "https://api.threat-optic.com"

	// EnvAPIURL and EnvAPIKey are the environment variables read when no
	// flag is supplied.
	EnvAPIURL = "THREATOPTIC_API_URL"
	EnvAPIKey = "THREATOPTIC_API_KEY"

	// The config file holds a bearer credential, so it stays owner-only.
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600

	// maskedKeyLength keeps the "top_" prefix plus eight characters, which
	// is enough to identify a key without exposing the secret.
	maskedKeyLength = 12
)

// Config is the on-disk configuration file.
type Config struct {
	APIURL string `yaml:"api_url,omitempty"`
	APIKey string `yaml:"api_key,omitempty"`
}

// Source identifies where a resolved value came from.
type Source string

const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "environment"
	SourceFile    Source = "config file"
	SourceDefault Source = "default"
	SourceNone    Source = "not set"
)

// Resolved is the effective configuration for a single command invocation.
type Resolved struct {
	APIURL       string
	APIURLSource Source
	APIKey       string
	APIKeySource Source
}

// Dir returns the directory holding the CLI configuration.
func Dir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "threatoptic"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".config", "threatoptic"), nil
}

// Path returns the full path to the configuration file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads the configuration file. A missing file is not an error.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	cfg.APIURL = strings.TrimSpace(cfg.APIURL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	return cfg, nil
}

// Save writes the configuration file with owner-only permissions. The write
// goes to a temporary file first so an interrupted save cannot truncate an
// existing credential.
func Save(cfg Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Chmod(dir, dirPerm); err != nil {
		return fmt.Errorf("secure %s: %w", dir, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, filePerm); err != nil {
		return fmt.Errorf("secure %s: %w", tmpName, err)
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Resolve applies the precedence rules: flag, then environment, then file,
// then the built-in default.
func Resolve(flagAPIURL, flagAPIKey string, file Config) Resolved {
	resolved := Resolved{
		APIURL:       DefaultAPIURL,
		APIURLSource: SourceDefault,
		APIKeySource: SourceNone,
	}

	switch {
	case strings.TrimSpace(flagAPIURL) != "":
		resolved.APIURL, resolved.APIURLSource = strings.TrimSpace(flagAPIURL), SourceFlag
	case strings.TrimSpace(os.Getenv(EnvAPIURL)) != "":
		resolved.APIURL, resolved.APIURLSource = strings.TrimSpace(os.Getenv(EnvAPIURL)), SourceEnv
	case file.APIURL != "":
		resolved.APIURL, resolved.APIURLSource = file.APIURL, SourceFile
	}

	switch {
	case strings.TrimSpace(flagAPIKey) != "":
		resolved.APIKey, resolved.APIKeySource = strings.TrimSpace(flagAPIKey), SourceFlag
	case strings.TrimSpace(os.Getenv(EnvAPIKey)) != "":
		resolved.APIKey, resolved.APIKeySource = strings.TrimSpace(os.Getenv(EnvAPIKey)), SourceEnv
	case file.APIKey != "":
		resolved.APIKey, resolved.APIKeySource = file.APIKey, SourceFile
	}

	resolved.APIURL = strings.TrimRight(resolved.APIURL, "/")
	return resolved
}

// MaskKey renders a credential for display. The full key is never returned.
func MaskKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= maskedKeyLength {
		return "..."
	}
	return key[:maskedKeyLength] + "..."
}
