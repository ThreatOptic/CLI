package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ThreatOptic/CLI/internal/api"
	"github.com/ThreatOptic/CLI/internal/config"
)

// fakeClient records how commands used the API without touching the network.
type fakeClient struct {
	user        *api.User
	checkResult *api.CheckResult
	err         error

	constructed int
	calls       int
	gotBaseURL  string
	gotAPIKey   string
	gotQuery    string
}

func (f *fakeClient) WhoAmI(context.Context) (*api.User, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.user, nil
}

func (f *fakeClient) Check(_ context.Context, query string) (*api.CheckResult, error) {
	f.calls++
	f.gotQuery = query
	if f.err != nil {
		return nil, f.err
	}
	return f.checkResult, nil
}

type harness struct {
	root   *cobra.Command
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	client *fakeClient
}

// newHarness builds a root command backed by a fake client and an isolated
// config directory.
func newHarness(t *testing.T, fake *fakeClient, promptKey string) *harness {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvAPIURL, "")
	t.Setenv(config.EnvAPIKey, "")

	opts := &options{
		newClient: func(baseURL, apiKey string) client {
			fake.constructed++
			fake.gotBaseURL = baseURL
			fake.gotAPIKey = apiKey
			return fake
		},
		readKey: func(*cobra.Command) (string, error) { return promptKey, nil },
	}

	root := newRoot("test-version", opts)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)

	return &harness{root: root, stdout: stdout, stderr: stderr, client: fake}
}

func (h *harness) run(t *testing.T, args ...string) error {
	t.Helper()
	h.root.SetArgs(args)
	return h.root.Execute()
}

func testUser() *api.User {
	return &api.User{
		ID:          "0192f3a1",
		Email:       "dev@example.com",
		Roles:       []string{"member"},
		Permissions: []string{"domains:read"},
	}
}

func TestVersionNeedsNoCredential(t *testing.T) {
	h := newHarness(t, &fakeClient{}, "")

	if err := h.run(t, "version"); err != nil {
		t.Fatalf("version error: %v", err)
	}

	if got := strings.TrimSpace(h.stdout.String()); got != "test-version" {
		t.Errorf("stdout = %q, want the version string", got)
	}
	if h.client.constructed != 0 {
		t.Error("version must not contact the API")
	}
}

func TestVersionJSON(t *testing.T) {
	h := newHarness(t, &fakeClient{}, "")

	if err := h.run(t, "version", "--json"); err != nil {
		t.Fatalf("version --json error: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(h.stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (%q)", err, h.stdout.String())
	}
	if payload["version"] != "test-version" {
		t.Errorf("version = %q, want test-version", payload["version"])
	}
}

func TestWhoamiHumanOutput(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "")

	if err := h.run(t, "whoami", "--api-key", "top_secretkey1234"); err != nil {
		t.Fatalf("whoami error: %v", err)
	}

	out := h.stdout.String()
	for _, want := range []string{"dev@example.com", "0192f3a1", "member"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout %q missing %q", out, want)
		}
	}
}

func TestWhoamiJSONOutput(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "")

	if err := h.run(t, "whoami", "--json", "--api-key", "top_secretkey1234"); err != nil {
		t.Fatalf("whoami --json error: %v", err)
	}

	var payload struct {
		ID    string   `json:"id"`
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (%q)", err, h.stdout.String())
	}
	if payload.Email != "dev@example.com" || payload.ID != "0192f3a1" {
		t.Errorf("payload = %+v, want the fake user", payload)
	}
	if len(payload.Roles) != 1 || payload.Roles[0] != "member" {
		t.Errorf("roles = %v, want [member]", payload.Roles)
	}
}

func TestWhoamiWithoutCredentialMakesNoRequest(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "")

	err := h.run(t, "whoami")
	if err == nil {
		t.Fatal("whoami without a credential should fail")
	}
	if !errors.Is(err, errNoAPIKey) {
		t.Errorf("error = %v, want errNoAPIKey", err)
	}
	if h.client.constructed != 0 {
		t.Error("no API client should be built without a credential")
	}
	if h.stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on failure", h.stdout.String())
	}
}

func TestWhoamiUnauthorizedSuggestsLogin(t *testing.T) {
	rejected := &api.Error{StatusCode: http.StatusUnauthorized, Message: "Invalid API key"}
	h := newHarness(t, &fakeClient{err: rejected}, "")

	err := h.run(t, "whoami", "--api-key", "top_badkey123456")
	if err == nil {
		t.Fatal("whoami with a rejected key should fail")
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Errorf("error = %q, want guidance to run auth login", err.Error())
	}
}

func TestWhoamiUsesResolvedURLAndKey(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "")

	if err := h.run(t, "whoami", "--api-url", "https://api.example.com/", "--api-key", "top_secretkey1234"); err != nil {
		t.Fatalf("whoami error: %v", err)
	}

	if h.client.gotBaseURL != "https://api.example.com" {
		t.Errorf("base URL = %q, want the flag value without a trailing slash", h.client.gotBaseURL)
	}
	if h.client.gotAPIKey != "top_secretkey1234" {
		t.Errorf("API key = %q, want the flag value", h.client.gotAPIKey)
	}
}

func TestAuthLoginStoresVerifiedKey(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "top_secretkey1234")

	if err := h.run(t, "auth", "login"); err != nil {
		t.Fatalf("auth login error: %v", err)
	}

	stored, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
	if stored.APIKey != "top_secretkey1234" {
		t.Errorf("stored key = %q, want the verified key", stored.APIKey)
	}
	if !strings.Contains(h.stdout.String(), "dev@example.com") {
		t.Errorf("stdout = %q, want the authenticated account", h.stdout.String())
	}
}

func TestAuthLoginDoesNotStoreRejectedKey(t *testing.T) {
	rejected := &api.Error{StatusCode: http.StatusUnauthorized, Message: "Invalid API key"}
	h := newHarness(t, &fakeClient{err: rejected}, "top_badkey123456")

	if err := h.run(t, "auth", "login"); err == nil {
		t.Fatal("auth login with a rejected key should fail")
	}

	stored, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
	if stored.APIKey != "" {
		t.Errorf("stored key = %q, want nothing written for a rejected key", stored.APIKey)
	}
}

func TestAuthLoginPromptsEvenWhenKeyAlreadyStored(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "top_replacementkey")

	if err := config.Save(config.Config{APIKey: "top_oldkey12345678"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := h.run(t, "auth", "login"); err != nil {
		t.Fatalf("auth login error: %v", err)
	}

	stored, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
	if stored.APIKey != "top_replacementkey" {
		t.Errorf("stored key = %q, want the newly entered key", stored.APIKey)
	}
}

func TestAuthStatusMasksStoredKey(t *testing.T) {
	const key = "top_secretkeyabcdefghij"
	h := newHarness(t, &fakeClient{user: testUser()}, "")

	if err := config.Save(config.Config{APIKey: key}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := h.run(t, "auth", "status"); err != nil {
		t.Fatalf("auth status error: %v", err)
	}

	out := h.stdout.String()
	if strings.Contains(out, key) {
		t.Errorf("stdout %q must not contain the full API key", out)
	}
	if !strings.Contains(out, config.MaskKey(key)) {
		t.Errorf("stdout %q should show the masked key", out)
	}
	if !strings.Contains(out, "dev@example.com") {
		t.Errorf("stdout %q should name the authenticated account", out)
	}
}

func TestAuthStatusJSONMasksStoredKey(t *testing.T) {
	const key = "top_secretkeyabcdefghij"
	h := newHarness(t, &fakeClient{user: testUser()}, "")

	if err := config.Save(config.Config{APIKey: key}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := h.run(t, "auth", "status", "--json"); err != nil {
		t.Fatalf("auth status --json error: %v", err)
	}

	out := h.stdout.String()
	if strings.Contains(out, key) {
		t.Errorf("JSON output %q must not contain the full API key", out)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (%q)", err, out)
	}
	if payload["email"] != "dev@example.com" {
		t.Errorf("email = %v, want dev@example.com", payload["email"])
	}
}

func TestAuthStatusReportsCredentialSource(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "")
	t.Setenv(config.EnvAPIKey, "top_fromenvironment12")

	if err := h.run(t, "auth", "status"); err != nil {
		t.Fatalf("auth status error: %v", err)
	}

	if !strings.Contains(h.stdout.String(), string(config.SourceEnv)) {
		t.Errorf("stdout = %q, want the credential source named", h.stdout.String())
	}
}

func TestAuthLogoutClearsStoredKey(t *testing.T) {
	h := newHarness(t, &fakeClient{user: testUser()}, "")

	if err := config.Save(config.Config{APIKey: "top_secretkey1234", APIURL: "https://api.example.com"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := h.run(t, "auth", "logout"); err != nil {
		t.Fatalf("auth logout error: %v", err)
	}

	stored, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
	if stored.APIKey != "" {
		t.Errorf("stored key = %q, want it cleared", stored.APIKey)
	}
	if stored.APIURL != "https://api.example.com" {
		t.Errorf("APIURL = %q, want the endpoint preserved", stored.APIURL)
	}
}

func TestAuthLogoutWithoutStoredKeySucceeds(t *testing.T) {
	h := newHarness(t, &fakeClient{}, "")

	if err := h.run(t, "auth", "logout"); err != nil {
		t.Fatalf("auth logout with no stored key should succeed, got: %v", err)
	}
}

func sampleCheckResult() *api.CheckResult {
	seen := "2026-01-15T00:00:00Z"
	return &api.CheckResult{
		Query:          "stripe.com",
		Kind:           "domain",
		Subject:        "stripe.com",
		SubjectKind:    "domain",
		SubjectPresent: true,
		MCP:            false,
		GeneratedCount: 120,
		Lookalikes: []api.CheckLookalike{
			{Name: "strlpe.com", Technique: "homoglyph", FirstSeenAt: &seen},
		},
	}
}

func TestCheckHumanOutput(t *testing.T) {
	h := newHarness(t, &fakeClient{checkResult: sampleCheckResult()}, "")

	if err := h.run(t, "check", "stripe.com", "--api-key", "top_secretkey1234"); err != nil {
		t.Fatalf("check error: %v", err)
	}

	out := h.stdout.String()
	for _, want := range []string{"kind: domain", "subject: stripe.com", "present: true", "mcp: false", "generated: 120", "strlpe.com", "homoglyph"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout %q missing %q", out, want)
		}
	}
	if h.client.gotQuery != "stripe.com" {
		t.Errorf("query = %q, want stripe.com", h.client.gotQuery)
	}
}

func TestCheckJSONOutput(t *testing.T) {
	h := newHarness(t, &fakeClient{checkResult: sampleCheckResult()}, "")

	if err := h.run(t, "check", "pypi:requests", "--json", "--api-key", "top_secretkey1234"); err != nil {
		t.Fatalf("check --json error: %v", err)
	}

	var payload api.CheckResult
	if err := json.Unmarshal(h.stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (%q)", err, h.stdout.String())
	}
	if payload.Subject != "stripe.com" || payload.GeneratedCount != 120 {
		t.Errorf("payload = %+v", payload)
	}
	if h.client.gotQuery != "pypi:requests" {
		t.Errorf("query = %q, want pypi:requests", h.client.gotQuery)
	}
}

func TestCheckWithoutCredentialMakesNoRequest(t *testing.T) {
	h := newHarness(t, &fakeClient{checkResult: sampleCheckResult()}, "")

	err := h.run(t, "check", "stripe.com")
	if err == nil {
		t.Fatal("check without a credential should fail")
	}
	if !errors.Is(err, errNoAPIKey) {
		t.Errorf("error = %v, want errNoAPIKey", err)
	}
	if h.client.constructed != 0 {
		t.Error("no API client should be built without a credential")
	}
}

func TestCheckUnauthorizedSuggestsLogin(t *testing.T) {
	rejected := &api.Error{StatusCode: http.StatusUnauthorized, Message: "Invalid API key"}
	h := newHarness(t, &fakeClient{err: rejected}, "")

	err := h.run(t, "check", "stripe.com", "--api-key", "top_badkey123456")
	if err == nil {
		t.Fatal("check with a rejected key should fail")
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Errorf("error = %q, want guidance to run auth login", err.Error())
	}
}

func TestCheckForbiddenSurfacesDetail(t *testing.T) {
	forbidden := &api.Error{StatusCode: http.StatusForbidden, Message: "Missing required permission(s): check:read"}
	h := newHarness(t, &fakeClient{err: forbidden}, "")

	err := h.run(t, "check", "stripe.com", "--api-key", "top_secretkey1234")
	if err == nil {
		t.Fatal("check with 403 should fail")
	}
	if !strings.Contains(err.Error(), "check:read") {
		t.Errorf("error = %q, want the API detail", err.Error())
	}
	if strings.Contains(err.Error(), "{") {
		t.Errorf("error %q leaked a raw body", err.Error())
	}
}

func TestCheckRequiresQueryArgument(t *testing.T) {
	h := newHarness(t, &fakeClient{checkResult: sampleCheckResult()}, "")

	err := h.run(t, "check", "--api-key", "top_secretkey1234")
	if err == nil {
		t.Fatal("check without a query should fail")
	}
	if h.client.calls != 0 {
		t.Error("check without a query must not call the API")
	}
}
