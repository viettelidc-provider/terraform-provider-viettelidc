// Package client provides the HTTP client for ViettelIDC IaC operations.
// All CSA endpoints accept HTTP POST with JSON
// body and return a uniform envelope { code, message, data }.
//
// The Client handles:
//   - Authentication via "authorization" + "x-authorization" headers
//   - Retry on transient failures (5xx and network errors)
//   - JSON marshalling/unmarshalling
//
// Response semantics (CSA envelope) are implemented in csa.go.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a CSA HTTP client. Construct with NewClient or
// NewClientWithTokens.
type Client struct {
	httpClient  *http.Client
	baseURL     string
	oldToken    string // sent as: Authorization: Bearer <oldToken>
	accessToken string // sent as: X-Authorization: Bearer <accessToken>
}

// NewClient builds a Client where the same token is used for both the
// Authorization and X-Authorization headers. Useful for tests and for the
// override path where the operator already holds a CMP token.
func NewClient(baseURL, token string) *Client {
	return NewClientWithTokens(baseURL, token, token)
}

// NewClientWithTokens builds a Client with separate tokens for the legacy
// Authorization header (oldToken / via-atm-login HS512 JWT) and the OAuth
// X-Authorization header (accessToken / cmp_internal RS256 JWT). This
// matches the live CMP frontend behaviour observed in HAR captures.
func NewClientWithTokens(baseURL, oldToken, accessToken string) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		baseURL:     strings.TrimRight(baseURL, "/"),
		oldToken:    oldToken,
		accessToken: accessToken,
	}
}

// WithHTTPClient overrides the underlying http.Client (useful for tests).
func (c *Client) WithHTTPClient(h *http.Client) *Client {
	c.httpClient = h
	return c
}

// Do executes a CSA request: POST {baseURL}{path} with JSON body and auth
// headers. It returns the raw response body bytes on HTTP 2xx. Retry policy
// is applied per retry.go (5xx + network errors only; 4xx including 401 fail
// immediately).
func (c *Client) Do(ctx context.Context, path string, body interface{}) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("api client: marshal body: %w", err)
	}

	url := c.baseURL + path

	var lastBody []byte
	op := func() (int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("authorization", "Bearer "+c.oldToken)
		req.Header.Set("x-authorization", "Bearer "+c.accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()

		lastBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, fmt.Errorf("api client: read body: %w", err)
		}
		return resp.StatusCode, nil
	}

	status, err := doWithRetry(ctx, op)
	if err != nil {
		if len(lastBody) > 0 {
			return nil, fmt.Errorf("%w; server response: %.512s", err, string(lastBody))
		}
		return nil, err
	}
	if status < 200 || status >= 300 {
		return lastBody, fmt.Errorf("api client: HTTP %d: %s", status, string(lastBody))
	}
	return lastBody, nil
}

// DoMethod executes an HTTP request with the given method. Unlike Do, the
// body may be nil (suitable for GET requests). path is appended to baseURL.
// Returns raw response bytes on 2xx.
func (c *Client) DoMethod(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("api client: marshal body: %w", err)
		}
	}

	url := c.baseURL + path

	var lastBody []byte
	op := func() (int, error) {
		var bodyReader io.Reader
		if len(payload) > 0 {
			bodyReader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return 0, err
		}
		if len(payload) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("authorization", "Bearer "+c.oldToken)
		req.Header.Set("x-authorization", "Bearer "+c.accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()

		lastBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, fmt.Errorf("api client: read body: %w", err)
		}
		return resp.StatusCode, nil
	}

	status, err := doWithRetry(ctx, op)
	if err != nil {
		if len(lastBody) > 0 {
			return nil, fmt.Errorf("%w; server response: %.512s", err, string(lastBody))
		}
		return nil, err
	}
	if status < 200 || status >= 300 {
		return lastBody, fmt.Errorf("api client: HTTP %d: %s", status, string(lastBody))
	}
	return lastBody, nil
}
