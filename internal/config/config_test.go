package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points config at a throwaway directory for the duration of a test.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv(EnvAPIURL, "")
	t.Setenv(EnvAPIKey, "")
	return home
}

func TestPathHonorsXDGConfigHome(t *testing.T) {
	home := isolate(t)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}

	want := filepath.Join(home, "threatoptic", "config.yaml")
	if path != want {
		t.Errorf("Path() = %q, want %q", path, want)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with no file should succeed, got error: %v", err)
	}
	if cfg.APIKey != "" || cfg.APIURL != "" {
		t.Errorf("Load() = %+v, want zero Config", cfg)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	isolate(t)

	want := Config{APIURL: "https://api.example.com", APIKey: "top_secretkey1234"}
	if err := Save(want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestSaveUsesOwnerOnlyPermissions(t *testing.T) {
	isolate(t)

	if err := Save(Config{APIKey: "top_secretkey1234"}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file mode = %o, want 600", perm)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("config dir mode = %o, want 700", perm)
	}
}

func TestSaveLeavesNoTemporaryFiles(t *testing.T) {
	isolate(t)

	if err := Save(Config{APIKey: "top_secretkey1234"}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("config dir contains %v, want only config.yaml", names)
	}
}

func TestResolvePrecedence(t *testing.T) {
	file := Config{APIURL: "https://file.example.com", APIKey: "top_fromfile1234"}

	tests := []struct {
		name       string
		flagURL    string
		flagKey    string
		envURL     string
		envKey     string
		wantURL    string
		wantKey    string
		wantURLSrc Source
		wantKeySrc Source
	}{
		{
			name:       "flag beats environment and file",
			flagURL:    "https://flag.example.com",
			flagKey:    "top_fromflag1234",
			envURL:     "https://env.example.com",
			envKey:     "top_fromenv12345",
			wantURL:    "https://flag.example.com",
			wantKey:    "top_fromflag1234",
			wantURLSrc: SourceFlag,
			wantKeySrc: SourceFlag,
		},
		{
			name:       "environment beats file",
			envURL:     "https://env.example.com",
			envKey:     "top_fromenv12345",
			wantURL:    "https://env.example.com",
			wantKey:    "top_fromenv12345",
			wantURLSrc: SourceEnv,
			wantKeySrc: SourceEnv,
		},
		{
			name:       "file used when nothing else is set",
			wantURL:    "https://file.example.com",
			wantKey:    "top_fromfile1234",
			wantURLSrc: SourceFile,
			wantKeySrc: SourceFile,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(EnvAPIURL, testCase.envURL)
			t.Setenv(EnvAPIKey, testCase.envKey)

			got := Resolve(testCase.flagURL, testCase.flagKey, file)

			if got.APIURL != testCase.wantURL {
				t.Errorf("APIURL = %q, want %q", got.APIURL, testCase.wantURL)
			}
			if got.APIKey != testCase.wantKey {
				t.Errorf("APIKey = %q, want %q", got.APIKey, testCase.wantKey)
			}
			if got.APIURLSource != testCase.wantURLSrc {
				t.Errorf("APIURLSource = %q, want %q", got.APIURLSource, testCase.wantURLSrc)
			}
			if got.APIKeySource != testCase.wantKeySrc {
				t.Errorf("APIKeySource = %q, want %q", got.APIKeySource, testCase.wantKeySrc)
			}
		})
	}
}

func TestResolveDefaultsWhenNothingConfigured(t *testing.T) {
	t.Setenv(EnvAPIURL, "")
	t.Setenv(EnvAPIKey, "")

	got := Resolve("", "", Config{})

	if DefaultAPIURL != "https://api.threat-optic.com" {
		t.Errorf("DefaultAPIURL = %q, want production API", DefaultAPIURL)
	}
	if got.APIURL != DefaultAPIURL {
		t.Errorf("APIURL = %q, want %q", got.APIURL, DefaultAPIURL)
	}
	if got.APIURLSource != SourceDefault {
		t.Errorf("APIURLSource = %q, want %q", got.APIURLSource, SourceDefault)
	}
	if got.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", got.APIKey)
	}
	if got.APIKeySource != SourceNone {
		t.Errorf("APIKeySource = %q, want %q", got.APIKeySource, SourceNone)
	}
}

func TestResolveTrimsTrailingSlashFromURL(t *testing.T) {
	t.Setenv(EnvAPIURL, "")
	t.Setenv(EnvAPIKey, "")

	got := Resolve("https://api.example.com/", "", Config{})

	if got.APIURL != "https://api.example.com" {
		t.Errorf("APIURL = %q, want trailing slash removed", got.APIURL)
	}
}

func TestMaskKeyNeverRevealsFullKey(t *testing.T) {
	const key = "top_abcdefghijklmnopqrstuvwxyz"

	masked := MaskKey(key)

	if strings.Contains(masked, key) {
		t.Fatalf("MaskKey(%q) = %q, must not contain the full key", key, masked)
	}
	if !strings.HasPrefix(masked, "top_abcdefgh") {
		t.Errorf("MaskKey() = %q, want a recognizable prefix", masked)
	}
	if len(masked) >= len(key) {
		t.Errorf("MaskKey() = %q, want shorter than the key", masked)
	}
}

func TestMaskKeyEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "empty key", key: "", want: ""},
		{name: "whitespace only", key: "   ", want: ""},
		{name: "short key fully hidden", key: "top_abc", want: "..."},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := MaskKey(testCase.key); got != testCase.want {
				t.Errorf("MaskKey(%q) = %q, want %q", testCase.key, got, testCase.want)
			}
		})
	}
}
