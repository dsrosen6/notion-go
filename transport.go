package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// request describes one API call. It is the Go equivalent of the JavaScript
// SDK's EndpointDefinition: which parameters go in the path, the query string,
// and the body is decided by the calling endpoint method, not by reflection.
type request struct {
	// method is an HTTP method constant.
	method string
	// path is relative to the API root, with IDs already escaped, e.g.
	// "blocks/abc123/children".
	path string
	// query holds query-string parameters. Repeated keys are sent repeated, as
	// filter_properties requires.
	query url.Values
	// body is marshaled as the JSON request body. A nil body sends no body and
	// no Content-Type; a non-nil body always sends one, even if it marshals to
	// {}, matching serializeBody in Client.ts:345-362.
	body any
}

// do sends r, retrying per the client's [RetryConfig], and decodes a successful
// response into out. A nil out discards the response body.
func (c *Client) do(ctx context.Context, r request, out any, opts ...RequestOption) error {
	if err := validatePath(r.path); err != nil {
		return err
	}

	cfg := requestConfig{
		auth:   c.auth,
		query:  url.Values{},
		header: http.Header{},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	u := c.baseURL.JoinPath(r.path)
	q := u.Query()
	for key, values := range r.query {
		for _, v := range values {
			q.Add(key, v)
		}
	}
	for key, values := range cfg.query {
		for _, v := range values {
			q.Add(key, v)
		}
	}
	u.RawQuery = q.Encode()

	// Buffered so each retry can re-send it.
	var body []byte
	if r.body != nil {
		var err error
		if body, err = json.Marshal(r.body); err != nil {
			return fmt.Errorf("notion: encoding request body: %w", err)
		}
	}

	raw, err := c.send(ctx, r.method, u.String(), body, cfg)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("notion: decoding response: %w", err)
	}
	return nil
}

// validatePath rejects a request path with an empty, ".", or ".." segment,
// whether literal or percent-encoded (e.g. "%2e%2e"). Such a segment would be
// collapsed when the URL is built and retarget the request at a different
// endpoint: Pages.Retrieve(ctx, "..") would otherwise send GET /v1.
//
// Ported from validateRequestPath, errors.ts:156-186, which rejects ".." in
// either form; the empty and "." cases are added because URL.JoinPath also
// collapses those.
func validatePath(path string) error {
	for seg := range strings.SplitSeq(path, "/") {
		decoded, err := url.PathUnescape(seg)
		if err != nil {
			decoded = seg
		}
		if seg == "" || decoded == "." || decoded == ".." {
			return fmt.Errorf("notion: %w: %q", ErrInvalidPathParameter, path)
		}
	}
	return nil
}

// send performs the request, retrying as the policy allows, and returns the raw
// body of the first successful response.
//
// Every failed attempt is logged at warn and the final success at info, per
// logRequestError (Client.ts:536-549) and the success log at Client.ts:499.
func (c *Client) send(ctx context.Context, method, urlStr string, body []byte, cfg requestConfig) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		raw, err := c.attempt(ctx, method, urlStr, body, cfg)
		if err == nil {
			c.logSuccess(ctx, method, urlStr, raw)
			return raw, nil
		}

		requestID, rayID := errorIDs(err)
		c.logger.WarnContext(ctx, "notion: request failed",
			slog.String("method", method),
			slog.String("url", urlStr),
			slog.Int("attempt", attempt+1),
			slog.String("error", err.Error()),
			slog.String("request_id", requestID),
			slog.String("ray_id", rayID),
		)

		if attempt >= c.retry.MaxRetries || !retryable(err, method) {
			return nil, err
		}

		delay := c.retryDelay(err, attempt)
		c.logger.WarnContext(ctx, "notion: retrying request",
			slog.String("method", method),
			slog.Int("attempt", attempt+1),
			slog.Duration("delay", delay),
			slog.String("error", err.Error()),
		)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.sleep(delay):
		}
	}
}

// logSuccess records a successful request at info. The response is parsed for
// its request_id only when the logger will emit the record, so the extra decode
// costs nothing in the default configuration. Mirrors extractRequestId,
// Client.ts:554-564.
func (c *Client) logSuccess(ctx context.Context, method, urlStr string, raw []byte) {
	if !c.logger.Enabled(ctx, slog.LevelInfo) {
		return
	}
	var meta struct {
		RequestID string `json:"request_id"`
	}
	// A body that is not a JSON object simply has no request ID to report.
	_ = json.Unmarshal(raw, &meta)
	c.logger.InfoContext(ctx, "notion: request success",
		slog.String("method", method),
		slog.String("url", urlStr),
		slog.String("request_id", meta.RequestID),
	)
}

// errorIDs extracts the Notion request ID and Cloudflare ray ID from a failed
// response error, or empty strings when err is not one.
func errorIDs(err error) (requestID, rayID string) {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr.RequestID, apiErr.RayID
	}
	if httpErr, ok := errors.AsType[*HTTPError](err); ok {
		return httpErr.RequestID, httpErr.RayID
	}
	return "", ""
}

// attempt performs a single request with no retrying.
func (c *Client) attempt(ctx context.Context, method, urlStr string, body []byte, cfg requestConfig) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, reader)
	if err != nil {
		return nil, fmt.Errorf("notion: building request: %w", err)
	}

	// Caller headers first, so the headers the client owns cannot be
	// overridden. Matches buildRequestHeaders, Client.ts:367-401.
	for key, values := range cfg.header {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	if cfg.auth != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.auth)
	}
	req.Header.Set("Notion-Version", c.notionVersion)
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	c.logger.DebugContext(ctx, "notion: request start",
		slog.String("method", method), slog.String("url", urlStr))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, transportError(ctx, "sending request", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, transportError(ctx, "reading response", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, parseError(resp.StatusCode, resp.Header, raw)
	}
	return raw, nil
}

// transportError classifies an error from sending the request or reading its
// body. The caller's context takes precedence: when it is done, its error is
// returned so the caller sees its own cancellation or deadline rather than
// [ErrRequestTimeout]. Otherwise a deadline or timeout is the client's own
// [http.Client.Timeout] and is reported as ErrRequestTimeout, wrapping the
// underlying error. Anything else is wrapped with the stage it failed in.
func transportError(ctx context.Context, stage string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("notion: %s: %w", stage, ctxErr)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrRequestTimeout, err)
	}
	if t, ok := errors.AsType[timeoutError](err); ok && t.Timeout() {
		return fmt.Errorf("%w: %w", ErrRequestTimeout, err)
	}
	return fmt.Errorf("notion: %s: %w", stage, err)
}

// timeoutError is the shape net and net/http errors use to flag a timeout.
type timeoutError interface {
	error
	Timeout() bool
}

// escapeID escapes an ID for interpolation into a request path. It neutralizes
// separators and traversal sequences, so a caller-supplied ID cannot retarget
// the request at a different endpoint. A bare "." or ".." is not escaped by
// [url.PathEscape]; [Client.do] rejects those with [ErrInvalidPathParameter].
//
// Notion returns property IDs already percent-encoded, such as "%3DAbc" for
// "=Abc", and expects them back in that form (the JavaScript SDK interpolates
// them into the path untouched). An ID that decodes cleanly is decoded first,
// so it is encoded exactly once on the wire rather than twice.
func escapeID(id string) string {
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	return url.PathEscape(id)
}
