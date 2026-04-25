// Package protonmail implements a ProtonMail API client.
package protonmail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

const Version = 3

const headerAPIVersion = "X-Pm-Apiversion"

// defaultBaseURL is the production Proton API endpoint used when WithBaseURL is not supplied.
const defaultBaseURL = "https://mail.proton.me/api"

// libraryVersion identifies this client library in the default User-Agent.
// Hardcoded for now; later issues may derive from build info.
const libraryVersion = "0.2.0-dev"

type resp struct {
	Code int
	*RawAPIError
}

func (r *resp) Err() error {
	if err := r.RawAPIError; err != nil {
		return &APIError{
			Code:    r.Code,
			Message: err.Message,
		}
	}
	return nil
}

type maybeError interface {
	Err() error
}

type RawAPIError struct {
	Message string `json:"Error"`
}

type APIError struct {
	Code    int
	Message string

	// HTTPError, if non-nil, carries the underlying HTTP-level failure
	// (status code, raw body) that produced this APIError. It is exposed
	// via Unwrap so errors.Is(err, ErrRateLimited) and friends can match
	// on HTTP status when Proton's application code is unknown.
	HTTPError *HTTPError
}

// Error implements error. It is nil-safe: calling Error() on a nil receiver
// returns "<nil>" instead of panicking.
func (err *APIError) Error() string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("[%v] %v", err.Code, err.Message)
}

type Timestamp int64

func (t Timestamp) Time() time.Time {
	return time.Unix(int64(t), 0)
}

// Client is a ProtonMail API client. Construct with NewClient; the zero value
// is not usable.
type Client struct {
	baseURL    string
	appVersion string
	userAgent  string
	debug      bool

	httpClient *http.Client
	reauth     func(ctx context.Context) error
	logger     *slog.Logger

	uid         string
	accessToken string
	keyRing     openpgp.EntityList
}

// NewClient constructs a Client. WithAppVersion is required; all other options
// have sensible defaults (base URL = https://mail.proton.me/api,
// User-Agent = "protonmail-go/<version>", logger discards, HTTP client =
// http.DefaultClient).
func NewClient(opts ...Option) (*Client, error) {
	c := &Client{
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
		userAgent:  fmt.Sprintf("protonmail-go/%s (+https://github.com/joestump/protonmail-go)", libraryVersion),
		logger:     slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	if c.appVersion == "" {
		return nil, errors.New("NewClient: app version is required (use WithAppVersion)")
	}
	return c, nil
}

func (c *Client) setRequestAuthorization(req *http.Request) {
	if c.uid != "" && c.accessToken != "" {
		req.Header.Set("X-Pm-Uid", c.uid)
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	if c.debug {
		log.Printf(">> %v %v\n", req.Method, req.URL.Path)
	}

	req.Header.Set("X-Pm-Appversion", c.appVersion)
	req.Header.Set(headerAPIVersion, strconv.Itoa(Version))
	c.setRequestAuthorization(req)
	return req, nil
}

func (c *Client) newJSONRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}
	b := buf.Bytes()

	req, err := c.newRequest(ctx, method, path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	if c.debug {
		log.Print(string(b))
	}

	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return req, nil
}

// do issues req and returns the response. On success (err == nil) the caller
// owns resp.Body and is responsible for draining and closing it. On error,
// do guarantees that any response body it obtained has been drained and
// closed, and returns (nil, err).
//
// A 401 with a configured ReAuth callback triggers a single re-auth + retry;
// the original 401 response body is drained and closed before the retry.
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", c.userAgent)

	// c.httpClient is guaranteed non-nil: NewClient defaults it to
	// http.DefaultClient and WithHTTPClient(nil) errors at construction.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// On transport error resp is nil per net/http contract; nothing to close.
		return nil, err
	}

	// Check if access token has expired. Retry is only safe if the request
	// body can be replayed: either there is no body, or GetBody was set so
	// the body can be re-read. If a 401 arrives with a non-replayable body
	// (req.Body != nil && GetBody == nil) we silently skip the retry and
	// surface the 401 to the caller — re-sending an already-consumed body
	// would send an empty body and corrupt the request. TODO #6: log this
	// case via the structured logger once it exists.
	_, hasAuth := req.Header["Authorization"]
	canRetry := req.Body == nil || req.GetBody != nil
	if resp.StatusCode == http.StatusUnauthorized && hasAuth && c.reauth != nil && canRetry {
		drainAndClose(resp.Body)
		c.accessToken = ""
		if err := c.reauth(ctx); err != nil {
			return nil, err
		}
		c.setRequestAuthorization(req) // Access token has changed
		if req.Body != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}
		return c.do(ctx, req)
	}

	return resp, nil
}

// drainAndClose drains rc up to a small cap so the connection can be reused,
// then closes it. Safe to call with a nil ReadCloser.
func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	// Cap drain to avoid unbounded reads on a hostile server.
	_, _ = io.Copy(io.Discard, io.LimitReader(rc, 64<<10))
	_ = rc.Close()
}

func (c *Client) doJSON(ctx context.Context, req *http.Request, respData interface{}) error {
	req.Header.Set("Accept", "application/json")

	if respData == nil {
		respData = new(resp)
	}

	httpResp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	// Status check FIRST. A non-2xx with a non-JSON body (e.g. an HTML error
	// page from an upstream proxy) used to surface as a confusing JSON decode
	// error; now it always becomes a typed error containing the status.
	if httpResp.StatusCode/100 != 2 {
		// Read up to 64KB of the error body for diagnostic context.
		bodyBytes, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64<<10))
		// Drain anything left so the connection can be reused. Cap the
		// drain to 1 MB so a hostile/slow-loris server can't pin this
		// goroutine forever (the deferred Close above never fires until
		// we return).
		_, _ = io.Copy(io.Discard, io.LimitReader(httpResp.Body, 1<<20))

		httpErr := &HTTPError{
			StatusCode: httpResp.StatusCode,
			Status:     httpResp.Status,
			Body:       bodyBytes,
		}

		// Best-effort: if the body parses as a Proton API error envelope,
		// surface the existing *APIError shape so callers that already match
		// on it keep working. Wire the HTTPError through so errors.Is can fall
		// through to the HTTP status when the Proton "Code" is unknown.
		var raw resp
		if json.Unmarshal(bodyBytes, &raw) == nil && raw.RawAPIError != nil && raw.RawAPIError.Message != "" {
			apiErr := &APIError{
				Code:      raw.Code,
				Message:   raw.RawAPIError.Message,
				HTTPError: httpErr,
			}
			if c.debug {
				log.Printf("<< %v %v: %v", req.Method, req.URL.Path, apiErr)
			}
			return apiErr
		}

		return httpErr
	}

	if err := json.NewDecoder(httpResp.Body).Decode(respData); err != nil {
		return err
	}

	if c.debug {
		log.Printf("<< %v %v", req.Method, req.URL.Path)
		log.Printf("%#v", respData)
	}

	if maybeError, ok := respData.(maybeError); ok {
		if err := maybeError.Err(); err != nil {
			log.Printf("request failed: %v %v: %v", req.Method, req.URL.String(), err)
			return err
		}
	}
	return nil
}
