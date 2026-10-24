// Package api is the HTTP client for the provider Open API (/open/v1) and the
// community site API (/v1). It knows about bearer tokens, the error envelope
// and multipart uploads; endpoint shapes live with the commands that use them.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// TokenSource returns the bearer token to send, or "" for anonymous calls.
type TokenSource func(ctx context.Context) (string, error)

// ErrNoToken is wrapped by a TokenSource when nothing is stored; optional-auth
// calls then proceed anonymously while required-auth calls fail.
var ErrNoToken = errors.New("not signed in")

type authMode int

const (
	authNone authMode = iota
	authRequired
	authOptional
)

// Client talks to one provider API base and one community site.
type Client struct {
	API       string // e.g. https://api.harperharbor.com (no trailing slash)
	Site      string // e.g. https://sukisuki.ai
	UserAgent string
	Token     TokenSource
	HTTP      *http.Client
}

// MaxIdleConnsPerHost is how many connections to one host stay open between
// requests. Concurrent media uploads use up to this many at once; the default
// transport keeps only two, so every other upload would open a new TLS
// connection.
const MaxIdleConnsPerHost = 16

// New builds a client with sane timeouts.
func New(api, site, userAgent string, token TokenSource) *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = MaxIdleConnsPerHost
	return &Client{
		API:       strings.TrimRight(api, "/"),
		Site:      strings.TrimRight(site, "/"),
		UserAgent: userAgent,
		Token:     token,
		HTTP:      &http.Client{Timeout: 120 * time.Second, Transport: tr},
	}
}

// Error is a non-2xx response. Code is the server's `error` string when the
// body was the documented envelope; Body keeps the raw text otherwise.
type Error struct {
	Status int
	Method string
	URL    string
	Code   string
	Detail any
	Body   string
	// Envelope preserves receipts carried beside a structured error. ErrorDetail
	// and the human error string continue to expose only the existing safe fields.
	Envelope json.RawMessage
	// RetryAfter is the body's retry_after in seconds (rate limits), or 0.
	RetryAfter int
}

func (e *Error) Error() string {
	target := e.URL
	if u, err := url.Parse(e.URL); err == nil {
		target = u.Path
	}
	switch {
	case e.Code != "" && e.Detail != nil:
		return fmt.Sprintf("%s %s: %d %s (%v)", e.Method, target, e.Status, e.Code, compact(e.Detail))
	case e.Code != "":
		return fmt.Sprintf("%s %s: %d %s", e.Method, target, e.Status, e.Code)
	case e.Body != "":
		return fmt.Sprintf("%s %s: %d %s", e.Method, target, e.Status, truncate(e.Body, 200))
	default:
		return fmt.Sprintf("%s %s: %d", e.Method, target, e.Status)
	}
}

// ErrorDetail exposes the structured body for --json output.
func (e *Error) ErrorDetail() any {
	m := map[string]any{"status": e.Status}
	if e.Code != "" {
		m["code"] = e.Code
	}
	if e.Detail != nil {
		m["detail"] = e.Detail
	}
	return m
}

// IsStatus reports whether err is an *Error with the given status.
func IsStatus(err error, status int) bool {
	var e *Error
	if ok := asError(err, &e); ok {
		return e.Status == status
	}
	return false
}

func asError(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// Open* call the provider Open API. path starts with "/" and is appended to
// "<API>/open/v1".
func (c *Client) OpenGet(ctx context.Context, path string, q url.Values, out any) error {
	return c.do(ctx, http.MethodGet, c.API+"/open/v1"+path, q, nil, out, authRequired)
}

func (c *Client) OpenPost(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, c.API+"/open/v1"+path, nil, body, out, authRequired)
}

func (c *Client) OpenPut(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, c.API+"/open/v1"+path, nil, body, out, authRequired)
}

// OpenPutLanguage is OpenPut with the provider's `language` header, which
// decides the language a new card is created in and the field limits it gets.
func (c *Client) OpenPutLanguage(ctx context.Context, path, language string, body, out any) error {
	return c.doWith(ctx, http.MethodPut, c.API+"/open/v1"+path, nil, body, out, authRequired, http.Header{"Language": {language}})
}

func (c *Client) OpenPatch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, c.API+"/open/v1"+path, nil, body, out, authRequired)
}

func (c *Client) OpenDelete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, c.API+"/open/v1"+path, nil, nil, out, authRequired)
}

// Provider calls an absolute path on the API host (OAuth, well-known).
func (c *Client) Provider(ctx context.Context, method, path string, q url.Values, body, out any, auth bool) error {
	mode := authNone
	if auth {
		mode = authRequired
	}
	return c.do(ctx, method, c.API+path, q, body, out, mode)
}

// SiteGet calls the community API. auth=true attaches the bearer when one is
// available and continues anonymously otherwise; false never sends one.
func (c *Client) SiteGet(ctx context.Context, path string, q url.Values, out any, auth bool) error {
	mode := authNone
	if auth {
		mode = authOptional
	}
	return c.do(ctx, http.MethodGet, c.Site+"/v1"+path, q, nil, out, mode)
}

// SitePost posts JSON to the community API; auth works as in SiteGet.
func (c *Client) SitePost(ctx context.Context, path string, body, out any, auth bool) error {
	mode := authNone
	if auth {
		mode = authOptional
	}
	return c.do(ctx, http.MethodPost, c.Site+"/v1"+path, nil, body, out, mode)
}

// SiteDo calls a member endpoint of the community API (bearer required).
func (c *Client) SiteDo(ctx context.Context, method, path string, body, out any) error {
	return c.do(ctx, method, c.Site+"/v1"+path, nil, body, out, authRequired)
}

// Form posts application/x-www-form-urlencoded (OAuth token endpoint).
func (c *Client) Form(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.API+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.send(req, out)
}

// UploadFile posts one multipart file with extra fields. contentType is the
// file part's declared type; empty means application/octet-stream. A file
// (anything with Stat) is streamed from disk with a known length instead of
// being read into memory first.
func (c *Client) UploadFile(ctx context.Context, path string, fields map[string][]string, fileField, fileName, contentType string, r io.Reader, out any) error {
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	for k, vs := range fields {
		for _, v := range vs {
			if err := mw.WriteField(k, v); err != nil {
				return err
			}
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, quoteEscaper.Replace(fileField), quoteEscaper.Replace(fileName)))
	h.Set("Content-Type", contentType)
	if _, err := mw.CreatePart(h); err != nil {
		return err
	}
	headLen := head.Len()
	if err := mw.Close(); err != nil {
		return err
	}
	// mw.Close wrote only the closing boundary after the part header.
	tail := append([]byte(nil), head.Bytes()[headLen:]...)
	head.Truncate(headLen)

	size := int64(-1)
	if st, ok := r.(interface{ Stat() (os.FileInfo, error) }); ok {
		if fi, err := st.Stat(); err == nil && fi.Mode().IsRegular() {
			size = fi.Size()
		}
	}
	if size < 0 {
		raw, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		r, size = bytes.NewReader(raw), int64(len(raw))
	}
	body := io.MultiReader(bytes.NewReader(head.Bytes()), io.LimitReader(r, size), bytes.NewReader(tail))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.API+"/open/v1"+path, body)
	if err != nil {
		return err
	}
	req.ContentLength = int64(head.Len()) + size + int64(len(tail))
	if file, ok := r.(*os.File); ok {
		// Lets the transport send the request again when a kept-alive
		// connection turns out to be closed, as it does for in-memory bodies.
		name, headBytes := file.Name(), head.Bytes()
		req.GetBody = func() (io.ReadCloser, error) {
			again, err := os.Open(name)
			if err != nil {
				return nil, err
			}
			body := io.MultiReader(bytes.NewReader(headBytes), io.LimitReader(again, size), bytes.NewReader(tail))
			return struct {
				io.Reader
				io.Closer
			}{body, again}, nil
		}
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := c.authorize(ctx, req, authRequired); err != nil {
		return err
	}
	return c.send(req, out)
}

// quoteEscaper escapes a multipart header parameter the way
// mime/multipart.CreateFormFile does.
var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// Download fetches a URL (used for media) and returns the body.
func (c *Client) Download(ctx context.Context, rawURL string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, "", &Error{Status: resp.StatusCode, Method: http.MethodGet, URL: rawURL}
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

func (c *Client) do(ctx context.Context, method, target string, q url.Values, body, out any, auth authMode) error {
	return c.doWith(ctx, method, target, q, body, out, auth, nil)
}

func (c *Client) doWith(ctx context.Context, method, target string, q url.Values, body, out any, auth authMode, header http.Header) error {
	if len(q) > 0 {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + q.Encode()
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range header {
		for _, v := range vs {
			if v != "" {
				req.Header.Add(k, v)
			}
		}
	}
	if err := c.authorize(ctx, req, auth); err != nil {
		return err
	}
	return c.send(req, out)
}

func (c *Client) authorize(ctx context.Context, req *http.Request, auth authMode) error {
	if auth == authNone || c.Token == nil {
		return nil
	}
	tok, err := c.Token(ctx)
	if err != nil {
		if auth == authOptional && errors.Is(err, ErrNoToken) {
			return nil
		}
		return err
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return nil
}

func (c *Client) send(req *http.Request, out any) error {
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		err := parseError(req, resp.StatusCode, raw)
		if e, ok := err.(*Error); ok && e.RetryAfter == 0 {
			if n, convErr := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); convErr == nil && n > 0 {
				e.RetryAfter = n
			}
		}
		return err
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if rm, ok := out.(*json.RawMessage); ok {
		*rm = append((*rm)[:0], raw...)
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", req.Method, req.URL.Path, err)
	}
	return nil
}

func parseError(req *http.Request, status int, raw []byte) error {
	e := &Error{Status: status, Method: req.Method, URL: req.URL.String(), Envelope: append(json.RawMessage(nil), raw...)}
	var env struct {
		Error  any    `json:"error"`
		Detail any    `json:"detail"`
		Msg    string `json:"msg"`
		Code   any    `json:"code"`
		// any, so an unexpected type never hides the error code.
		RetryAfter any `json:"retry_after"`
	}
	if json.Unmarshal(raw, &env) == nil {
		if n, ok := env.RetryAfter.(float64); ok && n > 0 {
			e.RetryAfter = int(n)
		}
		switch v := env.Error.(type) {
		case string:
			e.Code = v
		case map[string]any:
			if s, ok := v["code"].(string); ok {
				e.Code = s
			} else if s, ok := v["message"].(string); ok {
				e.Code = s
			}
			e.Detail = v
		}
		if e.Code == "" && env.Msg != "" {
			e.Code = env.Msg
		}
		if env.Detail != nil {
			e.Detail = env.Detail
		}
	}
	if e.Code == "" {
		e.Body = strings.TrimSpace(string(raw))
	}
	return e
}

func compact(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return truncate(string(raw), 300)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
