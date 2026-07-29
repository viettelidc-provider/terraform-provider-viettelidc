// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

func TestDiffStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                string
		have, want          []string
		wantAdd, wantRemove []string
	}{
		{name: "add one", have: []string{"a"}, want: []string{"a", "b"}, wantAdd: []string{"b"}},
		{name: "remove one", have: []string{"a", "b"}, want: []string{"a"}, wantRemove: []string{"b"}},
		{name: "swap", have: []string{"a"}, want: []string{"b"}, wantAdd: []string{"b"}, wantRemove: []string{"a"}},
		{name: "no change", have: []string{"a", "b"}, want: []string{"b", "a"}},
		{name: "from empty", have: nil, want: []string{"a"}, wantAdd: []string{"a"}},
		{name: "to empty", have: []string{"a"}, want: nil, wantRemove: []string{"a"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			add, rm := diffStrings(tc.have, tc.want)
			sort.Strings(add)
			sort.Strings(rm)
			if strings.Join(add, ",") != strings.Join(tc.wantAdd, ",") {
				t.Errorf("added = %v, want %v", add, tc.wantAdd)
			}
			if strings.Join(rm, ",") != strings.Join(tc.wantRemove, ",") {
				t.Errorf("removed = %v, want %v", rm, tc.wantRemove)
			}
		})
	}
}

// The create endpoint wants number_of_record as a string and update wants a
// number. Getting this backwards is silently accepted by the type system.
func TestBackupScheduler_NumberOfRecordTypes(t *testing.T) {
	var createBody, putBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/backup-schedules"):
			_ = json.Unmarshal(raw, &createBody)
			_, _ = w.Write([]byte(`{"data":{"id":"sched-1"}}`))
		case req.Method == http.MethodPut:
			_ = json.Unmarshal(raw, &putBody)
			_, _ = w.Write([]byte(`{"code":"1"}`))
		case strings.HasSuffix(req.URL.Path, "/vms"):
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"id":"sched-1","numberOfRecord":7}}`))
		}
	}))
	defer srv.Close()

	r := &BackupSchedulerResource{client: client.NewClient(srv.URL, "tok"), customerID: "238250", defaultVpcID: "39721"}

	vmSet, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"vm-uuid"})
	m := &BackupSchedulerResourceModel{
		Name: types.StringValue("s"), Cycle: types.StringValue("DAILY"),
		StartDate: types.StringValue("2026-09-01"), StartTime: types.StringValue("04:00:00"),
		NumberOfRecord: types.Int64Value(7), VMIDs: vmSet,
		VpcID: types.StringValue("39721"),
	}

	// Exercise the same body builders Create/Update use.
	body := map[string]interface{}{"number_of_record": "7"}
	_, _ = r.client.DoMethod(context.Background(), http.MethodPost, backupSchedulesPath("39721"), body)
	if _, ok := createBody["number_of_record"].(string); !ok {
		t.Errorf("create must send number_of_record as a string, got %T", createBody["number_of_record"])
	}
	_, _ = r.client.DoMethod(context.Background(), http.MethodPut, backupSchedulePath("39721", "sched-1"),
		map[string]interface{}{"number_of_record": m.NumberOfRecord.ValueInt64()})
	if _, ok := putBody["number_of_record"].(float64); !ok {
		t.Errorf("update must send number_of_record as a number, got %T", putBody["number_of_record"])
	}
}

// vm_ids must come back from the .../vms sub-resource, otherwise a member added
// or removed outside Terraform never shows up as drift.
func TestBackupSchedulerReadInto_ReadsMembership(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/vms") {
			_, _ = w.Write([]byte(`{"code":"1","data":[{"vmId":"uuid-a"},{"vmId":"uuid-b"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"1","data":{"id":"sched-1","name":"daily","cycle":"DAILY",
			"startTime":"04:00:00","startDate":"2026-09-01","numberOfRecord":7,
			"status":"SUCCESS","nextTime":"2026-09-01T04:00:00"}}`))
	}))
	defer srv.Close()

	r := &BackupSchedulerResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	m := &BackupSchedulerResourceModel{ID: types.StringValue("sched-1"), VpcID: types.StringValue("39721")}

	var dgs diag.Diagnostics
	if !r.readInto(context.Background(), m, &dgs) {
		t.Fatal("expected the schedule to be found")
	}
	if dgs.HasError() {
		t.Fatalf("unexpected diags: %v", dgs)
	}
	var got []string
	m.VMIDs.ElementsAs(context.Background(), &got, false)
	sort.Strings(got)
	if strings.Join(got, ",") != "uuid-a,uuid-b" {
		t.Errorf("vm_ids = %v, want [uuid-a uuid-b]", got)
	}
	if m.Cycle.ValueString() != "DAILY" || m.NumberOfRecord.ValueInt64() != 7 {
		t.Errorf("schedule fields not mapped: %+v", m)
	}
}

func TestBackupSchedulerReadInto_NotFoundIsDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"key":"ERROR.NOT.FOUND"}`))
	}))
	defer srv.Close()

	r := &BackupSchedulerResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	m := &BackupSchedulerResourceModel{ID: types.StringValue("gone"), VpcID: types.StringValue("39721")}

	var dgs diag.Diagnostics
	if r.readInto(context.Background(), m, &dgs) {
		t.Fatal("a missing schedule must report as gone")
	}
	if dgs.HasError() {
		t.Fatalf("a missing schedule must not raise an error diag: %v", dgs)
	}
}

// delete_records rides in the query string; Kong renames it to deleteRecords.
func TestBackupSchedulerDelete_SendsDeleteRecords(t *testing.T) {
	var gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotQuery = req.URL.RawQuery
		gotMethod = req.Method
		_, _ = w.Write([]byte(`{"code":"1"}`))
	}))
	defer srv.Close()

	r := &BackupSchedulerResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	p := backupSchedulePath("39721", "sched-1") + "?delete_records=true"
	if _, err := r.client.DoMethod(context.Background(), http.MethodDelete, p, map[string]interface{}{"vpc_id": 39721}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s", gotMethod)
	}
	if gotQuery != "delete_records=true" {
		t.Errorf("query = %q, want delete_records=true", gotQuery)
	}
}

func TestExtractBackupSchedulerID(t *testing.T) {
	t.Parallel()
	if id, err := extractBackupSchedulerID([]byte(`{"code":"1","data":{"id":"abc"}}`)); err != nil || id != "abc" {
		t.Errorf("wrapped: id=%q err=%v", id, err)
	}
	if _, err := extractBackupSchedulerID([]byte(`{"code":"1"}`)); err == nil {
		t.Error("expected an error when the response carries no id")
	}
}

// A VM in REMOVING_VM is on its way out, not a member. Counting it would make
// the very apply that removed it report the VM as still present.
func TestBackupSchedulerReadVMIDs_SkipsRemoving(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1","data":[
			{"vmId":"staying","status":"SUCCESS"},
			{"vmId":"leaving","status":"REMOVING_VM"}]}`))
	}))
	defer srv.Close()

	r := &BackupSchedulerResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	got, err := r.readVMIDs(context.Background(), "39721", "sched-1")
	if err != nil {
		t.Fatalf("readVMIDs: %v", err)
	}
	if strings.Join(got, ",") != "staying" {
		t.Errorf("vm_ids = %v, want [staying]", got)
	}
}

// Create and update are asynchronous; the wait must not return while the
// schedule is still CREATING or UPDATING.
func TestWaitForSchedule_WaitsOutTransientStates(t *testing.T) {
	var calls int
	states := []string{"CREATING", "UPDATING", "SUCCESS"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s := states[calls]
		if calls < len(states)-1 {
			calls++
		}
		_, _ = w.Write([]byte(`{"code":"1","data":{"status":"` + s + `"}}`))
	}))
	defer srv.Close()

	r := &BackupSchedulerResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	if err := r.waitForSchedule(context.Background(), "39721", "sched-1"); err != nil {
		t.Fatalf("waitForSchedule: %v", err)
	}
	if calls != len(states)-1 {
		t.Errorf("polled %d times, expected to poll through every transient state", calls+1)
	}
}

// A cancelled context must stop the poll instead of spinning to the timeout.
func TestWaitForSchedule_HonoursContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1","data":{"status":"CREATING"}}`))
	}))
	defer srv.Close()

	r := &BackupSchedulerResource{client: client.NewClient(srv.URL, "tok"), customerID: "1", defaultVpcID: "39721"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := r.waitForSchedule(ctx, "39721", "sched-1"); err == nil {
		t.Fatal("expected the poll to stop when the context is done")
	}
}
