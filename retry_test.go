package notion

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// errorHandler serves an API error body with the given code and status.
func errorHandler(status int, code APIErrorCode, calls *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"code":%q,"message":"boom"}`, code)
	}
}

func TestRetryMatrix(t *testing.T) {
	// Retryability is decided by the API error code and the HTTP method, not
	// by the status. Ported from canRetry, Client.ts:573-596.
	tests := []struct {
		code      APIErrorCode
		status    int
		method    string
		wantCalls int
	}{
		{CodeRateLimited, 429, http.MethodGet, 3},
		{CodeRateLimited, 429, http.MethodPost, 3},
		{CodeRateLimited, 429, http.MethodPatch, 3},
		{CodeServiceOverload, 529, http.MethodPost, 3},

		{CodeInternalServerError, 500, http.MethodGet, 3},
		{CodeInternalServerError, 500, http.MethodDelete, 3},
		{CodeInternalServerError, 500, http.MethodPost, 1},
		{CodeInternalServerError, 500, http.MethodPatch, 1},
		{CodeServiceUnavailable, 503, http.MethodGet, 3},
		{CodeServiceUnavailable, 503, http.MethodPost, 1},

		{CodeGatewayTimeout, 504, http.MethodGet, 1},
		{CodeConflictError, 409, http.MethodGet, 1},
		{CodeObjectNotFound, 404, http.MethodGet, 1},
		{CodeValidationError, 400, http.MethodPost, 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.code, tt.method), func(t *testing.T) {
			var calls int
			srv := httptest.NewServer(errorHandler(tt.status, tt.code, &calls))
			defer srv.Close()

			c, _ := testClient(t, srv)
			err := c.do(context.Background(), request{method: tt.method, path: "x"}, nil)
			if err == nil {
				t.Fatal("expected an error")
			}
			if calls != tt.wantCalls {
				t.Errorf("server called %d times, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestUnknownErrorCodeNotRetried(t *testing.T) {
	// A 500 with an HTML body comes from the edge proxy, parses as an
	// HTTPError rather than an APIError, and must not be retried.
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "<html>error</html>")
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if err := c.do(context.Background(), request{method: http.MethodGet, path: "x"}, nil); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("server called %d times, want 1", calls)
	}
}

func TestRetryEventuallySucceeds(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"code":"rate_limited","message":"slow down"}`)
			return
		}
		io.WriteString(w, `{"id":"ok"}`)
	}))
	defer srv.Close()

	c, delays := testClient(t, srv)
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(context.Background(), request{method: http.MethodGet, path: "x"}, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if out.ID != "ok" {
		t.Errorf("ID = %q, want ok", out.ID)
	}
	if len(*delays) != 2 {
		t.Errorf("slept %d times, want 2", len(*delays))
	}
}

func TestRetryBodyIsResent(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		if len(bodies) < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"code":"rate_limited","message":"slow down"}`)
			return
		}
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	err := c.do(context.Background(),
		request{method: http.MethodPost, path: "search", body: map[string]string{"query": "x"}}, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("got %d requests, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("retry sent %q, want the original %q", bodies[1], bodies[0])
	}
}

func TestBackoffBounds(t *testing.T) {
	// base = InitialDelay << attempt; the delay is uniform in [base/2, 1.5*base).
	tests := []struct {
		jitter float64
		want   []time.Duration
	}{
		{0.0, []time.Duration{500 * time.Millisecond, time.Second}},
		{0.5, []time.Duration{time.Second, 2 * time.Second}},
		{0.999, []time.Duration{1499 * time.Millisecond, 2998 * time.Millisecond}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("jitter=%v", tt.jitter), func(t *testing.T) {
			var calls int
			srv := httptest.NewServer(errorHandler(429, CodeRateLimited, &calls))
			defer srv.Close()

			c, delays := testClient(t, srv)
			c.jitter = func() float64 { return tt.jitter }
			c.do(context.Background(), request{method: http.MethodGet, path: "x"}, nil)

			if len(*delays) != len(tt.want) {
				t.Fatalf("slept %d times, want %d", len(*delays), len(tt.want))
			}
			for i, want := range tt.want {
				if (*delays)[i] != want {
					t.Errorf("delay %d = %v, want %v", i, (*delays)[i], want)
				}
			}
		})
	}
}

func TestRetryAfterHeaderWins(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"code":"rate_limited","message":"slow down"}`)
	}))
	defer srv.Close()

	c, delays := testClient(t, srv)
	c.do(context.Background(), request{method: http.MethodGet, path: "x"}, nil)

	for i, d := range *delays {
		if d != 7*time.Second {
			t.Errorf("delay %d = %v, want 7s from the Retry-After header", i, d)
		}
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "9999")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"code":"rate_limited","message":"slow down"}`)
	}))
	defer srv.Close()

	c, delays := testClient(t, srv)
	c.do(context.Background(), request{method: http.MethodGet, path: "x"}, nil)

	if (*delays)[0] != DefaultMaxRetryDelay {
		t.Errorf("delay = %v, want it capped at %v", (*delays)[0], DefaultMaxRetryDelay)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{"absent", "", 0, false},
		{"delta seconds", "120", 2 * time.Minute, true},
		{"zero seconds", "0", 0, true},
		{"negative seconds", "-5", 0, false},
		// parseInt(value, 10) semantics, Client.ts:630: the leading run of
		// digits is the value and the rest is ignored.
		{"fractional seconds truncate", "7.5", 7 * time.Second, true},
		{"trailing garbage is ignored", "120abc", 2 * time.Minute, true},
		{"leading whitespace is skipped", "  30", 30 * time.Second, true},
		{"negative with trailing garbage", "-5s", 0, false},
		{"http date in the future", "Thu, 03 Sep 2026 12:00:30 GMT", 30 * time.Second, true},
		{"http date in the past clamps to zero", "Thu, 03 Sep 2026 11:59:00 GMT", 0, true},
		{"garbage", "soon", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.value != "" {
				header.Set("Retry-After", tt.value)
			}
			got, ok := parseRetryAfter(header, now)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("delay = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithoutRetry(t *testing.T) {
	var calls int
	srv := httptest.NewServer(errorHandler(429, CodeRateLimited, &calls))
	defer srv.Close()

	c, _ := testClient(t, srv, WithoutRetry())
	c.do(context.Background(), request{method: http.MethodGet, path: "x"}, nil)

	if calls != 1 {
		t.Errorf("server called %d times, want 1", calls)
	}
}

func TestContextCancellationDuringRetry(t *testing.T) {
	var calls int
	srv := httptest.NewServer(errorHandler(429, CodeRateLimited, &calls))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := NewClient("token", WithBaseURL(srv.URL))
	// Cancel while the client is waiting out the backoff.
	c.sleep = func(d time.Duration) <-chan time.Time {
		cancel()
		return make(chan time.Time)
	}

	err := c.do(ctx, request{method: http.MethodGet, path: "x"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("server called %d times, want 1", calls)
	}
}
