// Package api is a thin client for the ThreatOptic HTTP API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout = 10 * time.Second

	// maxErrorBody bounds how much of an error response is parsed.
	maxErrorBody = 64 << 10
)

// Client talks to the ThreatOptic API using an API key or JWT bearer token.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New returns a client for baseURL authenticating with apiKey.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// Error is a non-2xx response from the API.
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return e.Message
}

// Unauthorized reports whether the request failed authentication.
func (e *Error) Unauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized
}

// User is the account described by GET /auth/me.
type User struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// WhoAmI returns the account the current credential belongs to.
func (c *Client) WhoAmI(ctx context.Context) (*User, error) {
	var user User
	if err := c.get(ctx, "/auth/me", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// CheckLookalike is one live lookalike in a Check result.
type CheckLookalike struct {
	Name        string  `json:"name"`
	Technique   string  `json:"technique"`
	Detail      *string `json:"detail"`
	FirstSeenAt *string `json:"first_seen_at"`
	Registry    *string `json:"registry"`
}

// CheckResult is the payload from GET /check.
type CheckResult struct {
	Query          string           `json:"query"`
	Kind           string           `json:"kind"`
	Subject        string           `json:"subject"`
	SubjectKind    string           `json:"subject_kind"`
	SubjectPresent bool             `json:"subject_present"`
	MCP            bool             `json:"mcp"`
	GeneratedCount int              `json:"generated_count"`
	Lookalikes     []CheckLookalike `json:"lookalikes"`
	Registries     []string         `json:"registries"`
}

// Check looks up live lookalikes for query.
func (c *Client) Check(ctx context.Context, query string) (*CheckResult, error) {
	path := "/check?q=" + url.QueryEscape(query)
	var result CheckResult
	if err := c.get(ctx, path, &result); err != nil {
		return nil, err
	}
	if result.Lookalikes == nil {
		result.Lookalikes = []CheckLookalike{}
	}
	return &result, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseError(resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response from %s: %w", path, err)
	}
	return nil
}

// parseError maps an API error body to an *Error. The response body itself is
// never surfaced to the user; only the "detail" field is.
func parseError(resp *http.Response) *Error {
	apiErr := &Error{StatusCode: resp.StatusCode}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil || len(body) == 0 {
		return apiErr
	}

	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Detail) == 0 {
		return apiErr
	}

	var detail string
	if err := json.Unmarshal(envelope.Detail, &detail); err == nil {
		apiErr.Message = strings.TrimSpace(detail)
		return apiErr
	}

	var entries []struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(envelope.Detail, &entries); err == nil {
		messages := make([]string, 0, len(entries))
		for _, entry := range entries {
			if msg := strings.TrimSpace(entry.Msg); msg != "" {
				messages = append(messages, msg)
			}
		}
		apiErr.Message = strings.Join(messages, "; ")
	}
	return apiErr
}
