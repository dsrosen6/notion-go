package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testClient returns a client pointed at srv whose retry delays elapse
// instantly, and records every delay it was asked to wait.
func testClient(t *testing.T, srv *httptest.Server, opts ...Option) (*Client, *[]time.Duration) {
	t.Helper()
	var delays []time.Duration
	opts = append([]Option{WithBaseURL(srv.URL)}, opts...)
	c := NewClient("secret-token", opts...)
	c.sleep = func(d time.Duration) <-chan time.Time {
		delays = append(delays, d)
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	c.jitter = func() float64 { return 0.5 }
	return c, &delays
}

func TestRequestHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	err := c.do(context.Background(),
		request{method: http.MethodGet, path: "users/me"},
		nil,
		// The client owns these three; a caller must not be able to take them over.
		WithHeader("Authorization", "Bearer attacker"),
		WithHeader("Notion-Version", "1999-01-01"),
		WithHeader("User-Agent", "not-us"),
		WithHeader("X-Custom", "kept"),
	)
	if err != nil {
		t.Fatalf("do: %v", err)
	}

	checks := map[string]string{
		"Authorization":  "Bearer secret-token",
		"Notion-Version": DefaultNotionVersion,
		"User-Agent":     "notion-go/" + Version,
		"X-Custom":       "kept",
	}
	for key, want := range checks {
		if got.Get(key) != want {
			t.Errorf("header %s = %q, want %q", key, got.Get(key), want)
		}
	}
	if len(got.Values("Authorization")) != 1 {
		t.Errorf("Authorization = %q, want exactly one value", got.Values("Authorization"))
	}
}

func TestRequestAuthOverride(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if err := c.do(context.Background(),
		request{method: http.MethodGet, path: "users/me"}, nil,
		WithRequestAuth("other-token"),
	); err != nil {
		t.Fatalf("do: %v", err)
	}
	if want := "Bearer other-token"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestQueryEncoding(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	err := c.do(context.Background(), request{
		method: http.MethodGet,
		path:   "pages/abc",
		query:  map[string][]string{"page_size": {"25"}},
	}, nil, WithFilterProperties("title", "AbC%3D", "%7CQ%3A", "AbC=", "100%"))
	if err != nil {
		t.Fatalf("do: %v", err)
	}

	// Array parameters repeat the key rather than using brackets or commas.
	// Notion hands out property IDs already percent-encoded, so an encoded ID
	// is decoded once before the query string is built (Client.ts:329-332)
	// and the server receives the plain ID; a plain ID passes through
	// unchanged; and one that is not valid percent-encoding is sent as given.
	want := []string{"title", "AbC=", "|Q:", "AbC=", "100%"}
	if !slices.Equal(got["filter_properties"], want) {
		t.Errorf("filter_properties = %q, want %q", got["filter_properties"], want)
	}
	if got.Get("page_size") != "25" {
		t.Errorf("page_size = %q, want 25", got.Get("page_size"))
	}
}

func TestRequestBody(t *testing.T) {
	tests := []struct {
		name            string
		body            any
		wantBody        string
		wantContentType string
	}{
		{
			// Endpoints with no body params send nothing at all.
			name:            "nil body sends no body",
			body:            nil,
			wantBody:        "",
			wantContentType: "",
		},
		{
			// Endpoints that declare body params always send an object, even
			// when the caller set none of them.
			name:            "empty struct still sends {}",
			body:            struct{}{},
			wantBody:        "{}",
			wantContentType: "application/json",
		},
		{
			name:            "populated body",
			body:            map[string]string{"query": "docs"},
			wantBody:        `{"query":"docs"}`,
			wantContentType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody []byte
			var gotContentType string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotBody, _ = io.ReadAll(r.Body)
				gotContentType = r.Header.Get("Content-Type")
				io.WriteString(w, `{}`)
			}))
			defer srv.Close()

			c, _ := testClient(t, srv)
			if err := c.do(context.Background(),
				request{method: http.MethodPost, path: "search", body: tt.body}, nil); err != nil {
				t.Fatalf("do: %v", err)
			}
			if string(gotBody) != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
			if gotContentType != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", gotContentType, tt.wantContentType)
			}
		})
	}
}

func TestResponseDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"object":"user","id":"u1"}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	var out struct {
		Object string `json:"object"`
		ID     string `json:"id"`
	}
	if err := c.do(context.Background(), request{method: http.MethodGet, path: "users/me"}, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if out.Object != "user" || out.ID != "u1" {
		t.Errorf("decoded %+v, want {user u1}", out)
	}
}

func TestErrorResponseMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Notion-Request-Id", "req-9")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"code":"object_not_found","message":"Could not find page."}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	err := c.do(context.Background(), request{method: http.MethodGet, path: "pages/missing"}, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T (%v), want *APIError", err, err)
	}
	if apiErr.Code != CodeObjectNotFound {
		t.Errorf("Code = %q, want %q", apiErr.Code, CodeObjectNotFound)
	}
	if apiErr.RequestID != "req-9" {
		t.Errorf("RequestID = %q, want req-9", apiErr.RequestID)
	}
	if !IsNotFound(err) {
		t.Error("IsNotFound = false, want true")
	}
}

// Notion hands out property IDs that are already percent-encoded and expects
// them back that way, so escapeID must not encode the percent signs again.
func TestEscapeIDPercentEncoded(t *testing.T) {
	cases := map[string]string{
		"zN%3BQ":                               "zN%3BQ", // ";" encoded once, as Notion sent it
		"%3DAbc":                               "=Abc",   // "=" needs no escaping in a path segment
		"title":                                "title",
		"1429989f-e8ac-4eff-bc8f-57f56486db54": "1429989f-e8ac-4eff-bc8f-57f56486db54",
		"a/b":                                  "a%2Fb",
		"%2F":                                  "%2F",    // decodes to "/", which is re-escaped
		"100%":                                 "100%25", // not valid encoding, escaped as literal text
		"..%2F..%2Fadmin":                      "..%2F..%2Fadmin",
	}
	for in, want := range cases {
		if got := escapeID(in); got != want {
			t.Errorf("escapeID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPathEscaping(t *testing.T) {
	var gotPath string
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPath = r.URL.EscapedPath()
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	// A traversal sequence must not be able to retarget the request.
	if err := c.do(context.Background(),
		request{method: http.MethodGet, path: "pages/" + escapeID("../../admin")}, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotPath != "/v1/pages/..%2F..%2Fadmin" {
		t.Errorf("path = %q, want the traversal escaped", gotPath)
	}

	// url.PathEscape leaves a bare "." or ".." alone, and URL.JoinPath would
	// collapse them: Pages.Retrieve(ctx, "..") would send GET /v1. Those
	// segments are rejected before any request is made, as are their
	// percent-encoded forms and an empty ID. Ported from validateRequestPath,
	// errors.ts:156-186.
	for _, id := range []string{"..", ".", ""} {
		if escapeID(id) != id {
			t.Fatalf("escapeID(%q) = %q; this test assumes it is a no-op", id, escapeID(id))
		}
	}
	rejected := []string{"..", ".", "%2e%2e", "%2E%2E", "%2e", ""}
	for _, id := range rejected {
		t.Run("rejects "+strconv.Quote(id), func(t *testing.T) {
			calls = 0
			err := c.do(context.Background(),
				request{method: http.MethodGet, path: "pages/" + id}, nil)
			if !errors.Is(err, ErrInvalidPathParameter) {
				t.Fatalf("err = %v, want ErrInvalidPathParameter", err)
			}
			if calls != 0 {
				t.Errorf("server received %d requests, want 0", calls)
			}
		})
	}
	// A segment that merely contains dots is an ordinary ID.
	calls = 0
	if err := c.do(context.Background(),
		request{method: http.MethodGet, path: "pages/" + escapeID("a..b")}, nil); err != nil {
		t.Fatalf("do(a..b): %v", err)
	}
	if calls != 1 || gotPath != "/v1/pages/a..b" {
		t.Errorf("path = %q after %d calls, want /v1/pages/a..b after 1", gotPath, calls)
	}
}

func TestTimeoutClassification(t *testing.T) {
	// The client's own timeout is ErrRequestTimeout whether it fires before
	// the headers arrive or while the body is being read. A deadline on the
	// caller's context is that context's error, never ErrRequestTimeout.
	const timeout = 30 * time.Millisecond

	stallHeaders := func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}
	stallBody := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"partial":`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		client      *http.Client
		ctxTimeout  time.Duration
		wantTimeout bool
	}{
		{
			name:        "client timeout before headers",
			handler:     stallHeaders,
			client:      &http.Client{Timeout: timeout},
			wantTimeout: true,
		},
		{
			name:        "client timeout while reading body",
			handler:     stallBody,
			client:      &http.Client{Timeout: timeout},
			wantTimeout: true,
		},
		{
			name:       "caller deadline before headers",
			handler:    stallHeaders,
			client:     &http.Client{},
			ctxTimeout: timeout,
		},
		{
			name:       "caller deadline while reading body",
			handler:    stallBody,
			client:     &http.Client{},
			ctxTimeout: timeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			ctx := context.Background()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}

			c, _ := testClient(t, srv, WithHTTPClient(tt.client), WithoutRetry())
			var out map[string]any
			err := c.do(ctx, request{method: http.MethodGet, path: "users/me"}, &out)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := errors.Is(err, ErrRequestTimeout); got != tt.wantTimeout {
				t.Errorf("errors.Is(err, ErrRequestTimeout) = %v, want %v (err: %v)", got, tt.wantTimeout, err)
			}
			if !tt.wantTimeout && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("errors.Is(err, context.DeadlineExceeded) = false, want true (err: %v)", err)
			}
		})
	}
}

func TestRequestLogging(t *testing.T) {
	// Every failed request is logged at warn and every success at info, with
	// the request ID when one is available. Ported from logRequestError
	// (Client.ts:536-549) and the success log at Client.ts:499.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/bad":
			w.Header().Set("X-Notion-Request-Id", "req-bad")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"code":"validation_error","message":"nope"}`)
		default:
			io.WriteString(w, `{"object":"user","request_id":"req-ok"}`)
		}
	}))
	defer srv.Close()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	c, _ := testClient(t, srv, WithLogger(logger), WithoutRetry())

	records := func() []map[string]any {
		t.Helper()
		var out []map[string]any
		for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			var rec map[string]any
			if err := json.Unmarshal(line, &rec); err != nil {
				t.Fatalf("unmarshal log line %q: %v", line, err)
			}
			out = append(out, rec)
		}
		return out
	}

	err := c.do(context.Background(), request{method: http.MethodPost, path: "bad"}, nil)
	if !IsValidationError(err) {
		t.Fatalf("err = %v, want validation_error", err)
	}
	recs := records()
	if len(recs) != 1 {
		t.Fatalf("got %d log records for a failure, want 1: %v", len(recs), recs)
	}
	warn := recs[0]
	if warn["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", warn["level"])
	}
	if warn["method"] != http.MethodPost || warn["attempt"] != float64(1) {
		t.Errorf("record = %v, want method POST and attempt 1", warn)
	}
	if warn["request_id"] != "req-bad" {
		t.Errorf("request_id = %v, want req-bad", warn["request_id"])
	}
	if msg, _ := warn["error"].(string); !strings.Contains(msg, "nope") {
		t.Errorf("error = %v, want it to carry the API message", warn["error"])
	}

	buf.Reset()
	if err := c.do(context.Background(), request{method: http.MethodGet, path: "users/me"}, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	recs = records()
	if len(recs) != 1 {
		t.Fatalf("got %d log records for a success, want 1: %v", len(recs), recs)
	}
	info := recs[0]
	if info["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", info["level"])
	}
	if info["method"] != http.MethodGet || info["request_id"] != "req-ok" {
		t.Errorf("record = %v, want method GET and request_id req-ok", info)
	}
}

func TestDecodeErrorNotRetried(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	var out map[string]any
	err := c.do(context.Background(), request{method: http.MethodGet, path: "users/me"}, &out)
	if err == nil {
		t.Fatal("expected a decode error")
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Errorf("err = %v, want a JSON syntax error", err)
	}
	if calls != 1 {
		t.Errorf("server called %d times, want 1 (decode errors are not retried)", calls)
	}
}
