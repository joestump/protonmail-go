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
}

func (err *APIError) Error() string {
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

func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", c.userAgent)

	// c.httpClient is guaranteed non-nil: NewClient defaults it to
	// http.DefaultClient and WithHTTPClient(nil) errors at construction.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return resp, err
	}

	// Check if access token has expired
	_, hasAuth := req.Header["Authorization"]
	canRetry := req.Body == nil || req.GetBody != nil
	if resp.StatusCode == http.StatusUnauthorized && hasAuth && c.reauth != nil && canRetry {
		resp.Body.Close()
		c.accessToken = ""
		if err := c.reauth(ctx); err != nil {
			return resp, err
		}
		c.setRequestAuthorization(req) // Access token has changed
		if req.Body != nil {
			body, err := req.GetBody()
			if err != nil {
				return resp, err
			}
			req.Body = body
		}
		return c.do(ctx, req)
	}

	return resp, nil
}

func (c *Client) doJSON(ctx context.Context, req *http.Request, respData interface{}) error {
	req.Header.Set("Accept", "application/json")

	if respData == nil {
		respData = new(resp)
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(respData); err != nil {
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
