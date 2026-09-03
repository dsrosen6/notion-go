package notion

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestParseError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		header     http.Header
		body       string
		wantAPI    bool
		wantCode   APIErrorCode
		wantReqID  string
		wantRayID  string
		wantMsgSub string
	}{
		{
			name:       "well-formed api error",
			status:     404,
			body:       `{"object":"error","status":404,"code":"object_not_found","message":"Could not find page."}`,
			wantAPI:    true,
			wantCode:   CodeObjectNotFound,
			wantMsgSub: "Could not find page.",
		},
		{
			name:      "request id falls back to header",
			status:    400,
			header:    http.Header{"X-Notion-Request-Id": {"req-from-header"}},
			body:      `{"code":"validation_error","message":"bad"}`,
			wantAPI:   true,
			wantCode:  CodeValidationError,
			wantReqID: "req-from-header",
		},
		{
			name:      "body request id wins over header",
			status:    400,
			header:    http.Header{"X-Notion-Request-Id": {"from-header"}},
			body:      `{"code":"validation_error","message":"bad","request_id":"from-body"}`,
			wantAPI:   true,
			wantCode:  CodeValidationError,
			wantReqID: "from-body",
		},
		{
			// Only code and message are validated, per parseAPIErrorResponseBody
			// in errors.ts:417-455. Oddly shaped optional fields must not
			// demote a genuine (here, retryable) API error to an HTTPError.
			name:      "malformed optional fields are tolerated",
			status:    429,
			header:    http.Header{"X-Notion-Request-Id": {"from-header"}},
			body:      `{"code":"rate_limited","message":"slow down","additional_data":"oops","request_id":12345}`,
			wantAPI:   true,
			wantCode:  CodeRateLimited,
			wantReqID: "from-header",
		},
		{
			// Retryability keys off the code, so an unrecognized one must not
			// masquerade as an APIError.
			name:    "unrecognized code is an HTTPError",
			status:  500,
			body:    `{"code":"some_new_code","message":"unknown"}`,
			wantAPI: false,
		},
		{
			name:    "missing message is an HTTPError",
			status:  500,
			body:    `{"code":"internal_server_error"}`,
			wantAPI: false,
		},
		{
			name:    "non-json body is an HTTPError",
			status:  502,
			body:    "<html>Bad Gateway</html>",
			wantAPI: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := tt.header
			if header == nil {
				header = http.Header{}
			}
			err := parseError(tt.status, header, []byte(tt.body))

			var apiErr *APIError
			if got := errors.As(err, &apiErr); got != tt.wantAPI {
				t.Fatalf("errors.As(*APIError) = %v, want %v (err: %v)", got, tt.wantAPI, err)
			}
			if !tt.wantAPI {
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) {
					t.Fatalf("expected *HTTPError, got %T", err)
				}
				if httpErr.Status != tt.status {
					t.Errorf("Status = %d, want %d", httpErr.Status, tt.status)
				}
				if string(httpErr.Body) != tt.body {
					t.Errorf("Body = %q, want %q", httpErr.Body, tt.body)
				}
				return
			}
			if apiErr.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", apiErr.Code, tt.wantCode)
			}
			if apiErr.Status != tt.status {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.status)
			}
			if tt.wantReqID != "" && apiErr.RequestID != tt.wantReqID {
				t.Errorf("RequestID = %q, want %q", apiErr.RequestID, tt.wantReqID)
			}
			if tt.wantMsgSub != "" && !strings.Contains(apiErr.Error(), tt.wantMsgSub) {
				t.Errorf("Error() = %q, want it to contain %q", apiErr.Error(), tt.wantMsgSub)
			}
		})
	}
}

func TestAPIErrorMalformedAdditionalData(t *testing.T) {
	body := `{"code":"validation_error","message":"bad","additional_data":["not","an","object"],"request_id":null}`
	err := parseError(400, http.Header{}, []byte(body))

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.Code != CodeValidationError {
		t.Errorf("Code = %q, want validation_error", apiErr.Code)
	}
	if apiErr.AdditionalData != nil {
		t.Errorf("AdditionalData = %#v, want nil for a non-object", apiErr.AdditionalData)
	}
	if apiErr.RequestID != "" {
		t.Errorf("RequestID = %q, want empty for a non-string", apiErr.RequestID)
	}
}

func TestAPIErrorAdditionalData(t *testing.T) {
	body := `{"code":"validation_error","message":"bad","additional_data":{"properties":["Name","Status"]}}`
	err := parseError(400, http.Header{}, []byte(body))

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	props, ok := apiErr.AdditionalData["properties"].([]any)
	if !ok {
		t.Fatalf("AdditionalData[properties] = %#v, want []any", apiErr.AdditionalData["properties"])
	}
	if len(props) != 2 || props[0] != "Name" {
		t.Errorf("properties = %#v, want [Name Status]", props)
	}
}

func TestHTTPErrorEdgeProxyMessage(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		header  http.Header
		want    []string
		notWant []string
	}{
		{
			name:   "ray id without request id names the edge proxy",
			status: 403,
			header: http.Header{
				"Cf-Ray":       {"8a1b2c3d4e5f"},
				"Content-Type": {"text/html"},
			},
			want: []string{"edge proxy", "content-type: text/html", "8a1b2c3d4e5f", "network security rule"},
		},
		{
			name:    "non-403 omits the security-rule note",
			status:  502,
			header:  http.Header{"Cf-Ray": {"deadbeef"}},
			want:    []string{"edge proxy", "deadbeef"},
			notWant: []string{"network security rule"},
		},
		{
			// A Notion request id means the API itself answered, so the edge
			// proxy explanation would be wrong.
			name:   "request id suppresses the edge proxy message",
			status: 500,
			header: http.Header{
				"Cf-Ray":              {"deadbeef"},
				"X-Notion-Request-Id": {"req-123"},
			},
			want:    []string{"status 500"},
			notWant: []string{"edge proxy"},
		},
		{
			name:    "no ray id is a plain message",
			status:  500,
			header:  http.Header{},
			want:    []string{"status 500"},
			notWant: []string{"edge proxy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseError(tt.status, tt.header, []byte("<html></html>"))
			msg := err.Error()
			for _, want := range tt.want {
				if !strings.Contains(msg, want) {
					t.Errorf("Error() = %q, want it to contain %q", msg, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(msg, notWant) {
					t.Errorf("Error() = %q, want it NOT to contain %q", msg, notWant)
				}
			}
		})
	}
}

func TestCodeHelpers(t *testing.T) {
	tests := []struct {
		code APIErrorCode
		fn   func(error) bool
		name string
	}{
		{CodeObjectNotFound, IsNotFound, "IsNotFound"},
		{CodeUnauthorized, IsUnauthorized, "IsUnauthorized"},
		{CodeRateLimited, IsRateLimited, "IsRateLimited"},
		{CodeValidationError, IsValidationError, "IsValidationError"},
		{CodeConflictError, IsConflict, "IsConflict"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &APIError{Code: tt.code}
			if !tt.fn(match) {
				t.Errorf("%s(%s) = false, want true", tt.name, tt.code)
			}
			if tt.fn(&APIError{Code: CodeInvalidJSON}) && tt.code != CodeInvalidJSON {
				t.Errorf("%s matched invalid_json", tt.name)
			}
			if tt.fn(errors.New("unrelated")) {
				t.Errorf("%s matched a non-API error", tt.name)
			}
			// Must survive wrapping.
			if !tt.fn(errors.Join(errors.New("context"), match)) {
				t.Errorf("%s did not unwrap", tt.name)
			}
		})
	}
}
