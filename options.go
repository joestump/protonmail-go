package protonmail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// Option configures a Client. Options are applied by NewClient in the order
// supplied; a non-nil error from any option aborts construction.
type Option func(*Client) error

// WithBaseURL sets the API base URL. The URL must use the https scheme; an
// http scheme is rejected unless the host is exactly one of "127.0.0.1",
// "::1", or "localhost" (case-insensitive), which is permitted as a carve-out
// so tests can target an httptest.Server. Other addresses in 127.0.0.0/8 and
// IPv4-mapped IPv6 forms are not accepted. Trailing slashes are stripped.
func WithBaseURL(u string) Option {
	return func(c *Client) error {
		parsed, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("WithBaseURL: %w", err)
		}
		switch parsed.Scheme {
		case "https":
			// allowed
		case "http":
			if !isLoopbackHost(parsed.Hostname()) {
				return errors.New("WithBaseURL: URL must use https (http allowed only for loopback addresses)")
			}
		default:
			return fmt.Errorf("WithBaseURL: unsupported scheme %q", parsed.Scheme)
		}
		c.baseURL = strings.TrimRight(u, "/")
		return nil
	}
}

// isLoopbackHost reports whether host is one of the exact loopback identifiers
// permitted by WithBaseURL: "127.0.0.1", "::1", or "localhost"
// (case-insensitive). It deliberately does not accept the broader 127.0.0.0/8
// range or IPv4-mapped IPv6 forms — production callers should not rely on
// those for tests, and a tighter check is a safer default.
func isLoopbackHost(host string) bool {
	switch {
	case host == "127.0.0.1":
		return true
	case host == "::1":
		return true
	case strings.EqualFold(host, "localhost"):
		return true
	}
	return false
}

// WithHTTPClient sets the HTTP client used for requests. Must be non-nil.
// Defaults to http.DefaultClient.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("WithHTTPClient: client must be non-nil")
		}
		c.httpClient = h
		return nil
	}
}

// WithAppVersion sets the X-Pm-Appversion header value. Required by NewClient.
func WithAppVersion(v string) Option {
	return func(c *Client) error {
		if v == "" {
			return errors.New("WithAppVersion: app version must not be empty")
		}
		c.appVersion = v
		return nil
	}
}

// WithUserAgent overrides the default User-Agent header. The default
// identifies this library ("protonmail-go/<version>"); callers may override
// to embed their own application identity.
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		if ua == "" {
			return errors.New("WithUserAgent: user agent must not be empty")
		}
		c.userAgent = ua
		return nil
	}
}

// WithReAuth installs a callback invoked when the API returns 401 on a
// retryable request. The callback should refresh the client's credentials
// (typically by calling AuthRefresh) before the request is retried.
func WithReAuth(fn func(context.Context) error) Option {
	return func(c *Client) error {
		if fn == nil {
			return errors.New("WithReAuth: callback must be non-nil")
		}
		c.reauth = fn
		return nil
	}
}

// WithLogger installs a structured logger. Pass slog.New(slog.DiscardHandler)
// (the default) to silence all output. Log-level routing of debug output is
// handled by the logger itself.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) error {
		if l == nil {
			return errors.New("WithLogger: logger must be non-nil")
		}
		c.logger = l
		return nil
	}
}
