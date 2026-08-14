package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhoAmISendsBearerTokenAndParsesUser(t *testing.T) {
	var gotAuth, gotPath, gotAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"0192f3a1","email":"dev@example.com","roles":["member"],"permissions":["domains:read"]}`))
	}))
	defer server.Close()

	user, err := New(server.URL, "top_secretkey1234").WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI() error: %v", err)
	}

	if gotAuth != "Bearer top_secretkey1234" {
		t.Errorf("Authorization header = %q, want Bearer token", gotAuth)
	}
	if gotPath != "/auth/me" {
		t.Errorf("request path = %q, want /auth/me", gotPath)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept header = %q, want application/json", gotAccept)
	}
	if user.ID != "0192f3a1" || user.Email != "dev@example.com" {
		t.Errorf("user = %+v, want id and email parsed", user)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "member" {
		t.Errorf("user.Roles = %v, want [member]", user.Roles)
	}
}

func TestNewTrimsTrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if _, err := New(server.URL+"/", "top_secretkey1234").WhoAmI(context.Background()); err != nil {
		t.Fatalf("WhoAmI() error: %v", err)
	}

	if gotPath != "/auth/me" {
		t.Errorf("request path = %q, want /auth/me without a doubled slash", gotPath)
	}
}

func TestWhoAmIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail":"Invalid or revoked API key"}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "top_badkey123456").WhoAmI(context.Background())
	if err == nil {
		t.Fatal("WhoAmI() with a rejected key should return an error")
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *api.Error", err)
	}
	if !apiErr.Unauthorized() {
		t.Errorf("Unauthorized() = false, want true for status %d", apiErr.StatusCode)
	}
	if apiErr.Error() != "Invalid or revoked API key" {
		t.Errorf("Error() = %q, want the API detail string", apiErr.Error())
	}
}

func TestErrorMessageFromResponseBody(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "string detail",
			status: http.StatusForbidden,
			body:   `{"detail":"Not enough permissions"}`,
			want:   "Not enough permissions",
		},
		{
			name:   "validation detail list",
			status: http.StatusUnprocessableEntity,
			body:   `{"detail":[{"msg":"field required"},{"msg":"value is not a valid email"}]}`,
			want:   "field required; value is not a valid email",
		},
		{
			name:   "empty body falls back to status",
			status: http.StatusInternalServerError,
			body:   ``,
			want:   "HTTP 500",
		},
		{
			name:   "non-JSON body falls back to status",
			status: http.StatusBadGateway,
			body:   `<html>upstream failure at /internal/path</html>`,
			want:   "HTTP 502",
		},
		{
			name:   "JSON without detail falls back to status",
			status: http.StatusNotFound,
			body:   `{"error":"missing"}`,
			want:   "HTTP 404",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(testCase.status)
				w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			_, err := New(server.URL, "top_secretkey1234").WhoAmI(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}

			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error type = %T, want *api.Error", err)
			}
			if apiErr.Error() != testCase.want {
				t.Errorf("Error() = %q, want %q", apiErr.Error(), testCase.want)
			}
			if apiErr.StatusCode != testCase.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, testCase.status)
			}
		})
	}
}

func TestErrorDoesNotLeakRawResponseBody(t *testing.T) {
	const secret = "upstream failure at /internal/path"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>" + secret + "</html>"))
	}))
	defer server.Close()

	_, err := New(server.URL, "top_secretkey1234").WhoAmI(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error %q leaks the raw response body", err.Error())
	}
}

func TestRequestHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New(server.URL, "top_secretkey1234").WhoAmI(ctx); err == nil {
		t.Fatal("WhoAmI() with a cancelled context should return an error")
	}
}

func TestCheckSendsQueryAndParsesResult(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"query":"stripe.com","kind":"domain","subject":"stripe.com","subject_kind":"domain","subject_present":true,"mcp":false,"generated_count":3,"lookalikes":[],"registries":null}`))
	}))
	defer server.Close()

	result, err := New(server.URL, "top_secretkey1234").Check(context.Background(), "stripe.com")
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if gotPath != "/check?q=stripe.com" {
		t.Errorf("request = %q, want /check?q=stripe.com", gotPath)
	}
	if result.Subject != "stripe.com" || result.GeneratedCount != 3 {
		t.Errorf("result = %+v", result)
	}
}

func TestClientHasBoundedTimeout(t *testing.T) {
	client := New("http://example.invalid", "top_secretkey1234")
	if client.http.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", client.http.Timeout, defaultTimeout)
	}
}

func TestCheckHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New(server.URL, "top_secretkey1234").Check(ctx, "stripe.com"); err == nil {
		t.Fatal("Check() with a cancelled context should return an error")
	}
}
