package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shortBackoffClient overrides retry constants via test-only http.Client
// timeout. The retry backoff itself uses package vars; tests rely on the
// fact that base = 1s but jitter is uniform [0, base) so on average ~500ms.
// To keep tests fast, use ctx with deadline and assert call counts only.

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient(srv.URL, "test-token")
	c.httpClient = &http.Client{Timeout: 5 * time.Second}
	return c
}

func TestDoSetsAuthHeaders(t *testing.T) {
	var gotAuth, gotXAuth, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("authorization")
		gotXAuth = r.Header.Get("x-authorization")
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"message":"OK","data":{}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Do(context.Background(), "/anything", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if gotXAuth != "Bearer test-token" {
		t.Errorf("x-authorization = %q, want %q", gotXAuth, "Bearer test-token")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
}

func TestDoPostsJSONBody(t *testing.T) {
	var gotBody []byte
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Do(context.Background(), "/op", map[string]string{"vpcId": "abc"})
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	var got map[string]string
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if got["vpcId"] != "abc" {
		t.Errorf("body vpcId = %q, want abc", got["vpcId"])
	}
}

func TestDoRetries5xxThenSucceeds(t *testing.T) {
	// Shorten backoff so test stays fast.
	withTestBackoff(t, 1*time.Millisecond, 5*time.Millisecond)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	body, err := c.Do(context.Background(), "/op", nil)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if !strings.Contains(string(body), `"code":0`) {
		t.Errorf("body = %s", body)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("call count = %d, want 3 (2 failures + 1 success)", got)
	}
}

func TestDo5xxExhaustsRetries(t *testing.T) {
	withTestBackoff(t, 1*time.Millisecond, 5*time.Millisecond)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Do(context.Background(), "/op", nil)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	// 1 initial + 3 retries = 4 calls
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("call count = %d, want 4 (1 initial + 3 retries)", got)
	}
}

func TestDo401NoRetry(t *testing.T) {
	withTestBackoff(t, 1*time.Millisecond, 5*time.Millisecond)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"unauthorized"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Do(context.Background(), "/op", nil)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("401 should not retry: call count = %d, want 1", got)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401: %v", err)
	}
}

func TestDo400NoRetry(t *testing.T) {
	withTestBackoff(t, 1*time.Millisecond, 5*time.Millisecond)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Do(context.Background(), "/op", nil)
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("400 should not retry: call count = %d, want 1", got)
	}
}

func TestDoMarshalError(t *testing.T) {
	c := NewClient("http://invalid", "tok")
	// channels can't be marshalled to JSON
	_, err := c.Do(context.Background(), "/op", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Errorf("expected marshal error, got %v", err)
	}
}

// withTestBackoff temporarily reduces retry backoff so tests run in ms.
// It uses t.Cleanup to restore originals.
func withTestBackoff(t *testing.T, initial, max time.Duration) {
	t.Helper()
	origInitial, origMax := initialBackoffOverride, maxBackoffOverride
	initialBackoffOverride = initial
	maxBackoffOverride = max
	t.Cleanup(func() {
		initialBackoffOverride = origInitial
		maxBackoffOverride = origMax
	})
}
