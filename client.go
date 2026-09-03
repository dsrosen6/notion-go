package notion

import (
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"time"
)

// Default retry and timeout settings, matching constants.ts.
const (
	// DefaultTimeout bounds a single request, including reading its body.
	DefaultTimeout = 60 * time.Second
	// DefaultMaxRetries is the number of retries after the first attempt, so
	// three total attempts.
	DefaultMaxRetries = 2
	// DefaultInitialRetryDelay is the base of the exponential backoff.
	DefaultInitialRetryDelay = time.Second
	// DefaultMaxRetryDelay caps any single backoff, including one derived from
	// a Retry-After header.
	DefaultMaxRetryDelay = 60 * time.Second
)

// RetryConfig controls how failed requests are retried. See [WithRetry].
type RetryConfig struct {
	// MaxRetries is the number of retries after the initial attempt. Zero
	// disables retrying.
	MaxRetries int
	// InitialDelay is the base delay, doubled on each attempt.
	InitialDelay time.Duration
	// MaxDelay caps any single delay.
	MaxDelay time.Duration
}

// Client is a Notion API client. It is safe for concurrent use.
//
// Create one with [NewClient].
type Client struct {
	httpClient    *http.Client
	baseURL       *url.URL
	auth          string
	notionVersion string
	userAgent     string
	logger        *slog.Logger
	retry         RetryConfig

	// Injected for tests: sleep waits out a retry delay and jitter returns a
	// value in [0, 1).
	sleep  func(d time.Duration) <-chan time.Time
	jitter func() float64

	// Users accesses the workspace's users.
	Users *UsersService
	// Pages creates and edits pages.
	Pages *PagesService
	// Blocks reads and edits page content.
	Blocks *BlocksService
	// Databases creates and edits databases.
	Databases *DatabasesService
	// DataSources queries and edits data sources.
	DataSources *DataSourcesService
	// Comments reads and writes comments.
	Comments *CommentsService
}

// Option configures a [Client].
type Option func(*Client)

// WithHTTPClient sets the underlying HTTP client. Use it to configure proxies,
// TLS, or connection pooling. A nil client keeps the default.
//
// The client's own Timeout is honored. When it is zero, requests are bounded
// only by the context passed to each call.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithBaseURL overrides the API root. The "/v1/" path segment is appended, so
// pass the origin only. Useful for pointing tests at an httptest.Server.
//
// A value that [url.Parse] rejects is silently ignored and the default root is
// kept.
func WithBaseURL(raw string) Option {
	return func(c *Client) {
		if u, err := url.Parse(raw); err == nil {
			c.baseURL = u.JoinPath("v1")
		}
	}
}

// WithNotionVersion overrides the Notion-Version header. Defaults to
// [DefaultNotionVersion]; change it only to pin an older API version.
func WithNotionVersion(v string) Option {
	return func(c *Client) { c.notionVersion = v }
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithLogger directs request logging to l. Requests log at debug, retries and
// failures at warn. Logging is discarded by default.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.logger = l }
}

// WithRetry replaces the retry configuration. Zero-valued fields fall back to
// the package defaults.
func WithRetry(rc RetryConfig) Option {
	return func(c *Client) {
		if rc.InitialDelay <= 0 {
			rc.InitialDelay = DefaultInitialRetryDelay
		}
		if rc.MaxDelay <= 0 {
			rc.MaxDelay = DefaultMaxRetryDelay
		}
		c.retry = rc
	}
}

// WithoutRetry disables retrying, so every request is attempted exactly once.
func WithoutRetry() Option {
	return func(c *Client) { c.retry.MaxRetries = 0 }
}

// NewClient returns a client authenticating with the given integration token.
//
//	client := notion.NewClient(os.Getenv("NOTION_TOKEN"))
func NewClient(token string, opts ...Option) *Client {
	base, _ := url.Parse(DefaultBaseURL)
	c := &Client{
		httpClient:    &http.Client{Timeout: DefaultTimeout},
		baseURL:       base.JoinPath("v1"),
		auth:          token,
		notionVersion: DefaultNotionVersion,
		userAgent:     "notion-go/" + Version,
		logger:        slog.New(slog.DiscardHandler),
		retry: RetryConfig{
			MaxRetries:   DefaultMaxRetries,
			InitialDelay: DefaultInitialRetryDelay,
			MaxDelay:     DefaultMaxRetryDelay,
		},
		sleep:  time.After,
		jitter: rand.Float64,
	}
	for _, opt := range opts {
		opt(c)
	}

	c.Users = &UsersService{c: c}
	c.Pages = &PagesService{c: c}
	c.Blocks = &BlocksService{c: c}
	c.Databases = &DatabasesService{c: c}
	c.DataSources = &DataSourcesService{c: c}
	c.Comments = &CommentsService{c: c}
	return c
}

// RequestOption configures a single request.
type RequestOption func(*requestConfig)

type requestConfig struct {
	auth   string
	query  url.Values
	header http.Header
}

// WithRequestAuth overrides the client's token for one request. Useful when a
// single client serves several integrations or OAuth grants.
func WithRequestAuth(token string) RequestOption {
	return func(rc *requestConfig) { rc.auth = token }
}

// WithFilterProperties limits the returned page properties to the given
// property IDs. Supported by the page retrieve, create, and update endpoints
// and by data source queries.
//
// Notion reports property IDs already percent-encoded (for example "%7CQ%3A"),
// and callers commonly paste them back verbatim. Each ID is decoded once before
// the query string is built so it is not encoded a second time, matching
// buildRequestUrl in Client.ts:329-332. An ID that is not valid percent-encoding
// is sent as given.
func WithFilterProperties(ids ...string) RequestOption {
	return func(rc *requestConfig) {
		for _, id := range ids {
			if decoded, err := url.PathUnescape(id); err == nil {
				id = decoded
			}
			rc.query.Add("filter_properties", id)
		}
	}
}

// WithHeader sets an extra request header. It cannot override Authorization,
// Notion-Version, or User-Agent, which the client always controls.
func WithHeader(key, value string) RequestOption {
	return func(rc *requestConfig) { rc.header.Set(key, value) }
}
