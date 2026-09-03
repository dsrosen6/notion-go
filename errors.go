package notion

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// APIErrorCode is the "code" field of a Notion API error response.
type APIErrorCode string

// The error codes the Notion API returns. Ported from errors.ts:8-23.
const (
	CodeUnauthorized        APIErrorCode = "unauthorized"
	CodeRestrictedResource  APIErrorCode = "restricted_resource"
	CodeObjectNotFound      APIErrorCode = "object_not_found"
	CodeRateLimited         APIErrorCode = "rate_limited"
	CodeInvalidJSON         APIErrorCode = "invalid_json"
	CodeInvalidRequestURL   APIErrorCode = "invalid_request_url"
	CodeInvalidRequest      APIErrorCode = "invalid_request"
	CodeInvalidBeta         APIErrorCode = "invalid_beta"
	CodeValidationError     APIErrorCode = "validation_error"
	CodeConflictError       APIErrorCode = "conflict_error"
	CodeInternalServerError APIErrorCode = "internal_server_error"
	CodeServiceOverload     APIErrorCode = "service_overload"
	CodeServiceUnavailable  APIErrorCode = "service_unavailable"
	CodeGatewayTimeout      APIErrorCode = "gateway_timeout"
)

// apiErrorCodes is the set of codes recognized as a well-formed API error. A
// response whose code is absent from this set is reported as an [HTTPError],
// which matters because retryability keys off the code, not the HTTP status.
var apiErrorCodes = map[APIErrorCode]bool{
	CodeUnauthorized:        true,
	CodeRestrictedResource:  true,
	CodeObjectNotFound:      true,
	CodeRateLimited:         true,
	CodeInvalidJSON:         true,
	CodeInvalidRequestURL:   true,
	CodeInvalidRequest:      true,
	CodeInvalidBeta:         true,
	CodeValidationError:     true,
	CodeConflictError:       true,
	CodeInternalServerError: true,
	CodeServiceOverload:     true,
	CodeServiceUnavailable:  true,
	CodeGatewayTimeout:      true,
}

// APIError is a well-formed error response from the Notion API: the body parsed
// as JSON with a recognized "code" and a "message".
//
// Narrow to it with [errors.As], then switch on Code:
//
//	var apiErr *notion.APIError
//	if errors.As(err, &apiErr) && apiErr.Code == notion.CodeObjectNotFound {
//		// ...
//	}
type APIError struct {
	// Code identifies the failure. Always one of the Code* constants.
	Code APIErrorCode
	// Message is the human-readable message from the API.
	Message string
	// Status is the HTTP status code.
	Status int
	// RequestID identifies the request in Notion's logs. Taken from the response
	// body, falling back to the x-notion-request-id header.
	RequestID string
	// RayID is the cf-ray header, present when Cloudflare handled the response.
	RayID string
	// AdditionalData carries error-specific detail. Its shape varies by Code.
	AdditionalData map[string]any
	// Body is the raw response body.
	Body []byte
	// Header is the full set of response headers.
	Header http.Header
}

func (e *APIError) Error() string {
	return fmt.Sprintf("notion: %s (%s, status %d)", e.Message, e.Code, e.Status)
}

// HTTPError is a failed response that is not a well-formed API error: a body
// that is not JSON, or one whose "code" this package does not recognize.
//
// Responses carrying a Cloudflare ray ID but no Notion request ID were answered
// by the edge proxy before reaching the API; Error says so, since the body in
// that case is usually HTML rather than anything actionable.
type HTTPError struct {
	Status int
	Body   []byte
	Header http.Header
	// RequestID is the x-notion-request-id header, if the response carried one.
	RequestID string
	// RayID is the cf-ray header, if the response carried one.
	RayID string
}

func (e *HTTPError) Error() string {
	base := fmt.Sprintf("notion: request failed with status %d", e.Status)
	// Ported from buildUnknownResponseMessage, errors.ts:393-415.
	if e.RayID == "" || e.RequestID != "" {
		return base
	}
	var contentType string
	if ct := e.Header.Get("content-type"); ct != "" {
		contentType = fmt.Sprintf(" (content-type: %s)", ct)
	}
	var blocked string
	if e.Status == http.StatusForbidden {
		blocked = " This may mean the request was blocked by a network security rule."
	}
	return fmt.Sprintf(
		"%s: returned by Notion's edge proxy before reaching the API%s.%s Cloudflare ray ID: %s. Include this ID when contacting Notion support.",
		base, contentType, blocked, e.RayID,
	)
}

// ErrRequestTimeout is returned when a request exceeds the client's own
// timeout, set through [WithHTTPClient]. Unlike the JavaScript SDK, which races
// a timer against the request and leaves it running, this cancels the
// underlying request.
//
// A deadline or cancellation on the caller's context is not reported as this
// error; it surfaces as that context's error instead.
var ErrRequestTimeout = errors.New("notion: request timed out")

// ErrInvalidPathParameter is returned before any request is sent when a path
// parameter, such as a page or block ID, is empty or would resolve to "." or
// "..", including percent-encoded forms like "%2e%2e". Such a value would
// retarget the request at a different endpoint. It corresponds to the
// JavaScript SDK's ClientErrorCode "invalid_path_parameter"
// (InvalidPathParameterError, errors.ts).
var ErrInvalidPathParameter = errors.New("notion: invalid path parameter")

// IsNotFound reports whether err is an object_not_found API error.
func IsNotFound(err error) bool { return hasCode(err, CodeObjectNotFound) }

// IsUnauthorized reports whether err is an unauthorized API error.
func IsUnauthorized(err error) bool { return hasCode(err, CodeUnauthorized) }

// IsRateLimited reports whether err is a rate_limited API error. The client
// already retries these; seeing one means the retries were exhausted.
func IsRateLimited(err error) bool { return hasCode(err, CodeRateLimited) }

// IsValidationError reports whether err is a validation_error API error.
func IsValidationError(err error) bool { return hasCode(err, CodeValidationError) }

// IsConflict reports whether err is a conflict_error API error, meaning the
// object was modified concurrently and the request can be retried.
func IsConflict(err error) bool { return hasCode(err, CodeConflictError) }

func hasCode(err error, code APIErrorCode) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

// errorBody is the JSON shape of a Notion API error response.
//
// Only code and message are typed. The other two are kept raw so that an
// unexpected shape (a string where an object is expected, a number where a
// string is) does not make the whole body fail to decode and turn a genuine,
// possibly retryable API error into an [HTTPError]. This matches
// parseAPIErrorResponseBody, errors.ts:417-455, which validates only that
// message and code are strings.
type errorBody struct {
	Code           APIErrorCode    `json:"code"`
	Message        string          `json:"message"`
	RequestID      json.RawMessage `json:"request_id"`
	AdditionalData json.RawMessage `json:"additional_data"`
}

// parseError builds the error for a non-2xx response. It returns an [APIError]
// when the body parses as a recognized API error and an [HTTPError] otherwise.
// Ported from buildRequestError, errors.ts:362-384.
func parseError(status int, header http.Header, body []byte) error {
	var parsed errorBody
	if err := json.Unmarshal(body, &parsed); err == nil &&
		parsed.Message != "" && apiErrorCodes[parsed.Code] {
		// request_id is used only when it is a JSON string; additional_data
		// only when it is a JSON object. Anything else is ignored.
		var requestID string
		_ = json.Unmarshal(parsed.RequestID, &requestID)
		if requestID == "" {
			requestID = header.Get("x-notion-request-id")
		}
		var additionalData map[string]any
		_ = json.Unmarshal(parsed.AdditionalData, &additionalData)
		return &APIError{
			Code:           parsed.Code,
			Message:        parsed.Message,
			Status:         status,
			RequestID:      requestID,
			RayID:          header.Get("cf-ray"),
			AdditionalData: additionalData,
			Body:           body,
			Header:         header,
		}
	}
	return &HTTPError{
		Status:    status,
		Body:      body,
		Header:    header,
		RequestID: header.Get("x-notion-request-id"),
		RayID:     header.Get("cf-ray"),
	}
}
