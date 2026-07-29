// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// A destroy failed with ERROR_POOL_IS_IN_OTHER_PROCESSING and then succeeded in
// 2s on a manual retry, so the call has to wait the busy state out itself.
func TestCallAPIRetryBusySucceedsAfterBusy(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"code":-1,"message":"ERROR_POOL_IS_IN_OTHER_PROCESSING"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL, "tok")
	_, diags := callAPIRetryBusy(context.Background(), c, "/x", map[string]interface{}{}, time.Minute)
	if diags.HasError() {
		t.Fatalf("expected success after retry, got %v", diags)
	}
	if calls != 2 {
		t.Fatalf("made %d calls, want 2", calls)
	}
}

// A real error must not be retried: it would turn every failed delete into a
// long stall before reporting what was wrong.
func TestCallAPIRetryBusyDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"code":-1,"message":"ERROR_LB_NOT_FOUND"}`))
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL, "tok")
	_, diags := callAPIRetryBusy(context.Background(), c, "/x", map[string]interface{}{}, time.Minute)
	if !diags.HasError() {
		t.Fatal("expected the error to surface")
	}
	if calls != 1 {
		t.Fatalf("made %d calls, want 1 — non-busy errors must not be retried", calls)
	}
}

// The busy state can outlast the budget; the error must still surface.
func TestCallAPIRetryBusyGivesUpAtTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":-1,"message":"ERROR_POOL_IS_IN_OTHER_PROCESSING"}`))
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL, "tok")
	_, diags := callAPIRetryBusy(context.Background(), c, "/x", map[string]interface{}{}, time.Nanosecond)
	if !diags.HasError() {
		t.Fatal("expected the busy error to surface once the budget ran out")
	}
}
