// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// The endpoint pages at whatever size we ask for and reports totalItems. Asking
// for one page only would silently truncate a VPC with a long backup history.
func TestBackupVMRecordsFetchAllPaging(t *testing.T) {
	t.Parallel()
	const total = backupVMRecordsPageSize + 7

	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		pages = append(pages, q.Get("page"))
		n := backupVMRecordsPageSize
		if q.Get("page") == "1" {
			n = total - backupVMRecordsPageSize
		}
		items := make([]string, 0, n)
		for i := 0; i < n; i++ {
			items = append(items, fmt.Sprintf(`{"id":"%s-%d","status":"AVAILABLE"}`, q.Get("page"), i))
		}
		fmt.Fprintf(w, `{"code":"1","items":[%s],"totalItems":%d}`, strings.Join(items, ","), total)
	}))
	defer srv.Close()

	d := &BackupVMRecordsDataSource{client: client.NewClient(srv.URL, "tok")}
	got, err := d.fetchAll(context.Background(), "39721")
	if err != nil {
		t.Fatalf("fetchAll: %v", err)
	}
	if len(got) != total {
		t.Fatalf("got %d records, want %d", len(got), total)
	}
	if strings.Join(pages, ",") != "0,1" {
		t.Fatalf("requested pages %v, want 0 then 1", pages)
	}
}

// A short first page ends the walk even when totalItems claims more, so a
// server that over-reports cannot spin the loop forever.
func TestBackupVMRecordsFetchAllStopsOnShortPage(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		fmt.Fprint(w, `{"code":"1","data":[{"id":"a"}],"totalItems":9999}`)
	}))
	defer srv.Close()

	d := &BackupVMRecordsDataSource{client: client.NewClient(srv.URL, "tok")}
	got, err := d.fetchAll(context.Background(), "39721")
	if err != nil {
		t.Fatalf("fetchAll: %v", err)
	}
	if len(got) != 1 || calls != 1 {
		t.Fatalf("got %d records in %d calls, want 1 and 1", len(got), calls)
	}
}
