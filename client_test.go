package notion

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithHTTPClientNilKeepsDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient("token", WithBaseURL(srv.URL), WithHTTPClient(nil))
	if c.httpClient == nil {
		t.Fatal("httpClient = nil, want the default kept")
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", c.httpClient.Timeout, DefaultTimeout)
	}
	if err := c.do(context.Background(), request{method: http.MethodGet, path: "users/me"}, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
}

func TestWithBaseURLInvalidIsIgnored(t *testing.T) {
	c := NewClient("token", WithBaseURL("http://[::1"))
	if got, want := c.baseURL.String(), DefaultBaseURL+"/v1"; got != want {
		t.Errorf("baseURL = %q, want %q", got, want)
	}
}
