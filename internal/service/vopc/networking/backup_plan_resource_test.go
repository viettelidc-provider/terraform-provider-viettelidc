// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

func TestExtractBackupScheduleID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, want string
		wantErr          bool
	}{
		{name: "wrapped in data", body: `{"code":"1","data":{"id":"91b29e9f-9a6d-4947-a1b3-d67110407561"}}`, want: "91b29e9f-9a6d-4947-a1b3-d67110407561"},
		{name: "top level", body: `{"id":"abc"}`, want: "abc"},
		{name: "missing", body: `{"code":"1"}`, wantErr: true},
		{name: "not json", body: `nope`, wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := extractBackupScheduleID([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got id %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A schedule that is gone must read as drift, not as a hard error — otherwise
// deleting one outside Terraform wedges every later plan.
func TestBackupPlanReadInto_NotFoundIsDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"title":"Not Found","key":"ERROR.NOT.FOUND"}`))
	}))
	defer srv.Close()
	t.Setenv("VIETTELIDC_BACKUP_BASE_URL", srv.URL)

	r := &BackupPlanResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	m := &BackupPlanResourceModel{ID: types.StringValue("missing"), VpcID: types.StringValue("39721")}

	var dgs diag.Diagnostics
	if r.readInto(context.Background(), m, &dgs) {
		t.Fatal("expected readInto to report the schedule as gone")
	}
	if dgs.HasError() {
		t.Fatalf("a missing schedule must not raise an error diag: %v", dgs)
	}
}

func TestBackupPlanReadInto_MapsFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1","key":"SUCCESS","data":{
			"id":"91b29e9f","name":"devaa","cycle":"DAILY","startTime":"03:00:00",
			"startDate":"2026-07-22","numberOfRecord":7,"status":"SUCCESS",
			"nextTime":"2026-07-23T03:00:00"}}`))
	}))
	defer srv.Close()
	t.Setenv("VIETTELIDC_BACKUP_BASE_URL", srv.URL)

	r := &BackupPlanResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	m := &BackupPlanResourceModel{ID: types.StringValue("91b29e9f"), VpcID: types.StringValue("39721")}

	var dgs diag.Diagnostics
	if !r.readInto(context.Background(), m, &dgs) {
		t.Fatal("expected the schedule to be found")
	}
	if dgs.HasError() {
		t.Fatalf("unexpected diags: %v", dgs)
	}
	if m.Cycle.ValueString() != "DAILY" {
		t.Errorf("cycle = %q, want DAILY", m.Cycle.ValueString())
	}
	if m.NumberOfRecord.ValueInt64() != 7 {
		t.Errorf("number_of_record = %d, want 7", m.NumberOfRecord.ValueInt64())
	}
	if m.NextTime.ValueString() != "2026-07-23T03:00:00" {
		t.Errorf("next_time = %q", m.NextTime.ValueString())
	}
}

// A UUID in vm_ids is passed through untouched; only numeric ids need lookups.
func TestResolveVMIDs_UUIDPassthrough(t *testing.T) {
	t.Parallel()
	r := &BackupPlanResource{}
	list, d := types.ListValueFrom(context.Background(), types.StringType,
		[]string{"78ec434d-8cc0-45c8-9bb6-8c3435a7f567"})
	if d.HasError() {
		t.Fatalf("building list: %v", d)
	}
	got, diags := r.resolveVMIDs(context.Background(), list, "39721")
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(got) != 1 || got[0] != "78ec434d-8cc0-45c8-9bb6-8c3435a7f567" {
		t.Errorf("got %v", got)
	}
}
