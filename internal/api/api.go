package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LaunchRequest struct {
	ProjectID   int64             `json:"projectId"`
	Name        string            `json:"name,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
}

type LaunchSession struct {
	LaunchUUID string `json:"launchUuid"`
	LaunchID   int64  `json:"launchId"`
}

// FileResult is one row of the per-file import ledger (mirrors ImportFileResultDto).
type FileResult struct {
	FileName string `json:"fileName"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Error    string `json:"error"`
}

// APIError is a non-2xx response. Status drives the retry policy; Message carries
// the server's verbatim message (extracted from the {code,message,...} body shape).
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// Retryable reports whether the request may be retried.
func (e *APIError) Retryable() bool { return e.Status >= 500 || e.Status == http.StatusTooManyRequests }

type Client struct {
	BaseURL   string
	Token     string
	UserAgent string
	JSON      *http.Client                     // create/finish/ledger
	Uploads   *http.Client                     // multipart uploads (long timeout)
	Sleep     func(time.Duration)              // injectable for tests
	Logf      func(format string, args ...any) // retry/skip warnings; nil = silent
}

func New(baseURL, token string, insecure bool, version string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Token:     token,
		UserAgent: "suluctl/" + version,
		JSON:      &http.Client{Timeout: 30 * time.Second, Transport: transport},
		Uploads:   &http.Client{Timeout: 10 * time.Minute, Transport: transport},
		Sleep:     time.Sleep,
	}
}

var backoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// withRetry runs fn up to len(backoff)+1 times. Non-retryable APIErrors abort immediately.
func (c *Client) withRetry(desc string, fn func() error) error {
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && !apiErr.Retryable() {
			return err
		}
		if attempt >= len(backoff) {
			return err
		}
		c.logf("%s failed (%v), retrying in %s", desc, err, backoff[attempt])
		c.Sleep(backoff[attempt])
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", c.UserAgent)
	return req, nil
}

// checkStatus turns a non-2xx response into *APIError (body capped at 8 KB).
func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	apiErr := &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
	// Two server body shapes (see GlobalExceptionHandler): generic errors carry the
	// human text under "message"; the 402 billing/license guards carry it under
	// "error" (no "message" key). Prefer message, fall back to error — mirrors the FE.
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		if parsed.Message != "" {
			apiErr.Code, apiErr.Message = parsed.Code, parsed.Message
		} else if parsed.Error != "" {
			apiErr.Code, apiErr.Message = parsed.Code, parsed.Error
		}
	}
	return apiErr
}

func (c *Client) postJSON(path string, payload, out any) error {
	return c.withRetry("POST "+path, func() error {
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		req, err := c.newRequest(http.MethodPost, path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.JSON.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := checkStatus(resp); err != nil {
			return err
		}
		if out == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	})
}

func (c *Client) CreateLaunch(req LaunchRequest) (LaunchSession, error) {
	var s LaunchSession
	err := c.postJSON("/api/import/launches", req, &s)
	return s, err
}

type finishRequest struct {
	ExecutionState string `json:"executionState,omitempty"`
}

func (c *Client) Finish(launchUUID, executionState string) error {
	return c.postJSON("/api/import/launches/"+url.PathEscape(launchUUID)+"/finish",
		finishRequest{ExecutionState: executionState}, nil)
}

func (c *Client) Ledger(launchUUID string) ([]FileResult, error) {
	var out []FileResult
	err := c.withRetry("GET ledger", func() error {
		req, err := c.newRequest(http.MethodGet,
			"/api/import/launches/"+url.PathEscape(launchUUID)+"/files", nil)
		if err != nil {
			return err
		}
		resp, err := c.JSON.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := checkStatus(resp); err != nil {
			return err
		}
		out = out[:0]
		return json.NewDecoder(resp.Body).Decode(&out)
	})
	return out, err
}

// UploadFiles streams the given files as one multipart request (parts named "files",
// base file names — the server identifies results by content, not path). The body is
// streamed via io.Pipe, never buffered whole in RAM. Files that fail to open are
// skipped with a warning: in watch mode a file can legitimately vanish between scan
// and send. Retries rebuild the request and re-read the files from disk.
func (c *Client) UploadFiles(launchUUID string, paths []string) ([]FileResult, error) {
	var out struct {
		Files []FileResult `json:"files"`
	}
	err := c.withRetry("upload batch", func() error {
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		go func() {
			pw.CloseWithError(c.writeParts(mw, paths))
		}()
		req, err := c.newRequest(http.MethodPost,
			"/api/import/launches/"+url.PathEscape(launchUUID)+"/files", pr)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := c.Uploads.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := checkStatus(resp); err != nil {
			return err
		}
		out.Files = nil
		return json.NewDecoder(resp.Body).Decode(&out)
	})
	return out.Files, err
}

// writeParts writes one part per readable file and closes the multipart trailer.
func (c *Client) writeParts(mw *multipart.Writer, paths []string) error {
	defer mw.Close()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			c.logf("skipping %s: %v", p, err)
			continue
		}
		part, err := mw.CreateFormFile("files", filepath.Base(p))
		if err == nil {
			_, err = io.Copy(part, f)
		}
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
