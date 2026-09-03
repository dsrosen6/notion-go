package notion

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// retryable reports whether err is worth retrying for the given HTTP method.
//
// The decision keys off the API error code, not the HTTP status. That
// distinction is load-bearing: a 500 carrying an HTML body from Notion's edge
// proxy parses as an [HTTPError], not an [APIError], and is not retried.
//
// Ported from canRetry, Client.ts:573-596.
func retryable(err error, method string) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.Code {
	case CodeRateLimited, CodeServiceOverload:
		// The server explicitly asked us to try again, so retry any method.
		return true
	case CodeInternalServerError, CodeServiceUnavailable:
		// A server error may still have applied the write, so retry only
		// methods that are safe to repeat.
		return method == http.MethodGet || method == http.MethodDelete
	default:
		return false
	}
}

// retryDelay returns how long to wait before the next attempt, which is
// zero-indexed.
//
// A usable Retry-After header wins outright. Otherwise the delay is exponential
// with jitter, distributed uniformly across [base/2, 1.5*base) where base is
// InitialDelay doubled per attempt. Either way it is capped at MaxDelay.
//
// Ported from calculateRetryDelay, Client.ts:603-616.
func (c *Client) retryDelay(err error, attempt int) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if d, ok := parseRetryAfter(apiErr.Header, time.Now()); ok {
			return min(d, c.retry.MaxDelay)
		}
	}
	base := c.retry.InitialDelay << attempt
	delay := time.Duration(float64(base)*c.jitter()) + base/2
	return min(delay, c.retry.MaxDelay)
}

// parseRetryAfter reads a Retry-After header in either supported form:
// delta-seconds ("120") or an HTTP-date. A date in the past yields zero, and an
// unparseable value yields ok=false so the caller falls back to backoff.
//
// The delta-seconds form is read the way JavaScript's parseInt(value, 10)
// reads it: leading whitespace is skipped and the leading run of digits is the
// value, so "7.5" is 7 seconds and "120abc" is 120. A negative value is
// rejected. Only a value with no leading digit is tried as an HTTP-date.
//
// Ported from parseRetryAfterHeader, Client.ts:623-643.
func parseRetryAfter(header http.Header, now time.Time) (time.Duration, bool) {
	value := header.Get("retry-after")
	if value == "" {
		return 0, false
	}
	trimmed := strings.TrimLeft(value, " \t")
	if strings.HasPrefix(trimmed, "-") {
		return 0, false
	}
	if digits := leadingDigits(trimmed); digits != "" {
		secs, err := strconv.ParseInt(digits, 10, 64)
		if err != nil || secs > maxRetryAfterSeconds {
			// Too large to represent; the caller caps it at MaxDelay anyway.
			return time.Duration(math.MaxInt64), true
		}
		return time.Duration(secs) * time.Second, true
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(date.Sub(now), 0), true
	}
	return 0, false
}

// maxRetryAfterSeconds is the largest delta-seconds value that fits in a
// time.Duration.
const maxRetryAfterSeconds = math.MaxInt64 / int64(time.Second)

// leadingDigits returns the run of ASCII digits at the start of s.
func leadingDigits(s string) string {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	return s[:end]
}
