package networking

// polling_test.go — tests for all async-status polling logic added to
// vpc, nat_gateway, load_balancer and backup_plan resources.
//
// Each test uses the fakeAPIServer helper defined in networking_test.go so
// no real network traffic is made.

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ─────────────────────────────────────────────────────────────────────────────
// pollUntilReady — used by VPC Create
// ─────────────────────────────────────────────────────────────────────────────

func TestPollUntilReady_ImmediateActive(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "ACTIVE"}
	})

	err := pollUntilReady(context.Background(), srv.newClient(), pathVPCDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"}, 30*time.Second)
	if err != nil {
		t.Fatalf("expected nil for ACTIVE status, got: %v", err)
	}
}

func TestPollUntilReady_ImmediateSuccess(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "success"}
	})

	err := pollUntilReady(context.Background(), srv.newClient(), pathVPCDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"}, 30*time.Second)
	if err != nil {
		t.Fatalf("expected nil for 'success' status, got: %v", err)
	}
}

func TestPollUntilReady_ErrorState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "error"}
	})

	err := pollUntilReady(context.Background(), srv.newClient(), pathVPCDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"}, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for 'error' status, got nil")
	}
	if !strings.Contains(err.Error(), "error state") {
		t.Errorf("error message should mention 'error state', got: %v", err)
	}
}

func TestPollUntilReady_FailedState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "failed"}
	})

	err := pollUntilReady(context.Background(), srv.newClient(), pathVPCDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"}, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for 'failed' status, got nil")
	}
}

// TestPollUntilReady_Timeout uses a zero timeout so the deadline is already
// past after the first (non-terminal) API response.
func TestPollUntilReady_Timeout(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "creating"}
	})

	err := pollUntilReady(context.Background(), srv.newClient(), pathVPCDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"}, 0)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should mention 'timed out', got: %v", err)
	}
}

func TestPollUntilReady_ContextCancelled(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "creating"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled — select on ctx.Done() fires immediately

	err := pollUntilReady(ctx, srv.newClient(), pathVPCDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"}, 30*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

// TestPollUntilReady_EventuallyActive simulates a creating→ACTIVE transition.
// The handler returns "creating" on the first call and "ACTIVE" on the second.
// Because pollUntilReady sleeps 3 s between retries, this test may take ~3 s.
func TestPollUntilReady_EventuallyActive(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	var calls int32
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			return float64(0), "ok", map[string]interface{}{"status": "creating"}
		}
		return float64(0), "ok", map[string]interface{}{"status": "ACTIVE"}
	})

	err := pollUntilReady(context.Background(), srv.newClient(), pathVPCDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"}, 30*time.Second)
	if err != nil {
		t.Fatalf("expected nil after eventual ACTIVE, got: %v", err)
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected >= 2 poll calls (creating then ACTIVE), got %d", atomic.LoadInt32(&calls))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// pollForStatus — used by Load Balancer Create
// ─────────────────────────────────────────────────────────────────────────────

func TestPollForStatus_ImmediatelyActive(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "ACTIVE"}
	})

	err := pollForStatus(context.Background(), srv.newClient(), pathLoadBalancerDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1", "vttLoadBalancerId": int64(99)},
		"status", []string{"ACTIVE", "active"}, 30*time.Second)
	if err != nil {
		t.Fatalf("expected nil for ACTIVE, got: %v", err)
	}
}

func TestPollForStatus_LowercaseActiveAccepted(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "active"}
	})

	err := pollForStatus(context.Background(), srv.newClient(), pathLoadBalancerDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1", "vttLoadBalancerId": int64(99)},
		"status", []string{"ACTIVE", "active"}, 30*time.Second)
	if err != nil {
		t.Fatalf("expected nil for lowercase 'active', got: %v", err)
	}
}

func TestPollForStatus_ErrorState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "ERROR"}
	})

	err := pollForStatus(context.Background(), srv.newClient(), pathLoadBalancerDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1", "vttLoadBalancerId": int64(99)},
		"status", []string{"ACTIVE"}, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for ERROR state, got nil")
	}
	if !strings.Contains(err.Error(), "error state") {
		t.Errorf("error should mention 'error state', got: %v", err)
	}
}

func TestPollForStatus_FailedState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "FAILED"}
	})

	err := pollForStatus(context.Background(), srv.newClient(), pathLoadBalancerDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1", "vttLoadBalancerId": int64(99)},
		"status", []string{"ACTIVE"}, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for FAILED state, got nil")
	}
}

func TestPollForStatus_Timeout(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "PENDING"}
	})

	err := pollForStatus(context.Background(), srv.newClient(), pathLoadBalancerDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1", "vttLoadBalancerId": int64(99)},
		"status", []string{"ACTIVE"}, 0)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should mention 'timed out', got: %v", err)
	}
}

func TestPollForStatus_ContextCancelled(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "PENDING"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pollForStatus(ctx, srv.newClient(), pathLoadBalancerDetail,
		map[string]interface{}{"vpc_id": "v1", "customer_id": "c1", "vttLoadBalancerId": int64(99)},
		"status", []string{"ACTIVE"}, 30*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NatGatewayResource.pollReady — list-based polling
// ─────────────────────────────────────────────────────────────────────────────

// natGatewayListResponse builds a CSA list response containing one NAT gateway
// with the given status. Helpers accept float64 IDs as JSON numbers decode to float64.
func natGatewayListResponse(id int64, status string) interface{} {
	return map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"id":          float64(id),
				"name":        "nat-test",
				"vttSubnetId": float64(1),
				"connectType": false,
				"nicIp":       "",
				"status":      status,
				"createdAt":   "",
			},
		},
	}
}

func TestNatGatewayPollReady_ImmediateActive(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "ACTIVE")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	if err := r.pollReady(context.Background(), m, 30*time.Second); err != nil {
		t.Fatalf("expected nil for ACTIVE status, got: %v", err)
	}
}

func TestNatGatewayPollReady_SuccessStatus(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "SUCCESS")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	if err := r.pollReady(context.Background(), m, 30*time.Second); err != nil {
		t.Fatalf("expected nil for SUCCESS status, got: %v", err)
	}
}

func TestNatGatewayPollReady_AvailableStatus(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "AVAILABLE")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	if err := r.pollReady(context.Background(), m, 30*time.Second); err != nil {
		t.Fatalf("expected nil for AVAILABLE status, got: %v", err)
	}
}

func TestNatGatewayPollReady_ErrorState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "ERROR")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	err := r.pollReady(context.Background(), m, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for ERROR state, got nil")
	}
	if !strings.Contains(err.Error(), "error state") {
		t.Errorf("error should mention 'error state', got: %v", err)
	}
}

func TestNatGatewayPollReady_FailedState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "FAILED")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	if err := r.pollReady(context.Background(), m, 30*time.Second); err == nil {
		t.Fatal("expected error for FAILED state, got nil")
	}
}

func TestNatGatewayPollReady_Timeout(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "CREATING")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	err := r.pollReady(context.Background(), m, 0)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should mention 'timed out', got: %v", err)
	}
}

func TestNatGatewayPollReady_ContextCancelled(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "CREATING")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	if err := r.pollReady(ctx, m, 30*time.Second); err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

// TestNatGatewayPollReady_EventuallyActive simulates creating→ACTIVE transition.
// NAT gateway pollReady sleeps 10 s between retries — this test may take ~10 s.
func TestNatGatewayPollReady_EventuallyActive(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	var calls int32
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			return float64(0), "ok", natGatewayListResponse(42, "CREATING")
		}
		return float64(0), "ok", natGatewayListResponse(42, "ACTIVE")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{ID: types.StringValue("42"), VpcID: types.StringValue("vpc-1")}

	if err := r.pollReady(context.Background(), m, 30*time.Second); err != nil {
		t.Fatalf("expected nil after eventual ACTIVE, got: %v", err)
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected >= 2 poll calls, got %d", atomic.LoadInt32(&calls))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BackupPlanResource.readAndMerge — status field propagation
// These tests verify that readAndMerge correctly reads the status field from
// the list response so the Create status-check switch can evaluate it.
// ─────────────────────────────────────────────────────────────────────────────

// backupPlanListResponse builds the nested envelope that BackupPlanResource.readAndMerge expects.
func backupPlanListResponse(id int64, status string) interface{} {
	content := map[string]interface{}{
		"id":               float64(id),
		"name":             "bp-test",
		"description":      "",
		"vttBackupCycleId": float64(1),
		"backupCycleName":  "daily",
		"startDayBackup":   "2025-01-01",
		"timeBackup":       "02:00",
		"numberOfRecord":   float64(7),
		"status":           status,
		"listVolume":       []interface{}{},
	}
	// The actual API response wraps data in {data:{content:[...]}}
	// but callAPI unwraps the outer envelope and leaves `data` as the inner payload.
	// readAndMerge json.Unmarshal's the inner payload into listResp which has
	// field "data.content" — so we need to encode as {"data":{"content":[...]}}
	return map[string]interface{}{
		"data": map[string]interface{}{
			"content": []interface{}{content},
		},
	}
}

func TestBackupPlanReadAndMerge_StatusPropagation(t *testing.T) {
	t.Parallel()
	statuses := []string{"ACTIVE", "ERROR", "CREATING", "FAILED"}
	for _, wantStatus := range statuses {
		wantStatus := wantStatus
		t.Run(wantStatus, func(t *testing.T) {
			t.Parallel()
			srv := newFakeAPI(t)
			srv.on(pathBackupPlanList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
				return float64(0), "ok", backupPlanListResponse(99, wantStatus)
			})

			r := &BackupPlanResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
			m := &BackupPlanResourceModel{
				ID:    types.StringValue("99"),
				VpcID: types.StringValue("vpc-1"),
			}

			var dgs diag.Diagnostics
			r.readAndMerge(context.Background(), m, &dgs)

			if dgs.HasError() {
				t.Fatalf("readAndMerge produced unexpected error diag: %v", dgs)
			}
			if m.Status.ValueString() != wantStatus {
				t.Errorf("expected status %q, got %q", wantStatus, m.Status.ValueString())
			}
		})
	}
}

func TestBackupPlanReadAndMerge_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathBackupPlanList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"data": map[string]interface{}{"content": []interface{}{}},
		}
	})

	r := &BackupPlanResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &BackupPlanResourceModel{
		ID:    types.StringValue("999"),
		VpcID: types.StringValue("vpc-1"),
	}

	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), m, &dgs)
	if !dgs.HasError() {
		t.Fatal("expected not-found error diag, got none")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCResource.readInto — basic smoke test for VPC detail read
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCResource_ReadInto_Success(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"id":          "vpc-1",
			"name":        "my-vpc",
			"cidrBlock":   "10.0.0.0/16",
			"description": "test vpc",
			"status":      "ACTIVE",
		}
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	m := &VPCResourceModel{
		ID: types.StringValue("vpc-1"),
	}

	var dgs diag.Diagnostics
	r.readInto(context.Background(), m, &dgs)
	if dgs.HasError() {
		t.Fatalf("unexpected error diag: %v", dgs)
	}
	if m.Name.ValueString() != "my-vpc" {
		t.Errorf("expected name 'my-vpc', got %q", m.Name.ValueString())
	}
	if m.CidrBlock.ValueString() != "10.0.0.0/16" {
		t.Errorf("expected cidr '10.0.0.0/16', got %q", m.CidrBlock.ValueString())
	}
}

func TestVPCResource_ReadInto_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "FAILURE", "VPC not found", nil
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	m := &VPCResourceModel{ID: types.StringValue("vpc-missing")}

	var dgs diag.Diagnostics
	found := r.readInto(context.Background(), m, &dgs)
	if found {
		t.Fatal("expected readInto to return false (drift) on not-found")
	}
	if dgs.HasError() {
		t.Fatalf("drift (not-found) should not produce error diag, got: %v", dgs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NatGatewayResource.readAndMerge — basic list-parse smoke test
// ─────────────────────────────────────────────────────────────────────────────

func TestNatGatewayReadAndMerge_Success(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"id":          float64(42),
					"name":        "nat-gw",
					"vttSubnetId": float64(5),
					"connectType": true,
					"nicIp":       "10.0.0.1",
					"status":      "ACTIVE",
					"createdAt":   "2025-01-01",
				},
			},
		}
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{
		ID:    types.StringValue("42"),
		VpcID: types.StringValue("vpc-1"),
	}

	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), m, &dgs)
	if dgs.HasError() {
		t.Fatalf("unexpected error: %v", dgs)
	}
	if m.Name.ValueString() != "nat-gw" {
		t.Errorf("expected name 'nat-gw', got %q", m.Name.ValueString())
	}
	if m.Status.ValueString() != "ACTIVE" {
		t.Errorf("expected status 'ACTIVE', got %q", m.Status.ValueString())
	}
	if m.FloatingIP.ValueString() != "10.0.0.1" {
		t.Errorf("expected floating_ip '10.0.0.1', got %q", m.FloatingIP.ValueString())
	}
}

func TestNatGatewayReadAndMerge_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"items": []interface{}{}}
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &NatGatewayResourceModel{
		ID:    types.StringValue("999"),
		VpcID: types.StringValue("vpc-1"),
	}

	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), m, &dgs)
	if !dgs.HasError() {
		t.Fatal("expected not-found error diag, got none")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration: VPC Create-then-poll happy path
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCCreatePollFlow_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "my-vpc" {
			t.Errorf("unexpected name in create body: %v", body["name"])
		}
		return float64(0), "ok", map[string]interface{}{"vpcId": "vpc-10"}
	})

	pollCalls := int32(0)
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		n := atomic.AddInt32(&pollCalls, 1)
		if n < 2 {
			return float64(0), "ok", map[string]interface{}{"status": "creating"}
		}
		return float64(0), "ok", map[string]interface{}{
			"id":          "vpc-10",
			"name":        "my-vpc",
			"cidrBlock":   "10.0.0.0/16",
			"description": "",
			"status":      "ACTIVE",
		}
	})

	c := srv.newClient()
	ctx := context.Background()

	// Step 1: Create
	createBody := map[string]interface{}{"name": "my-vpc", "cidr_block": "10.0.0.0/16", "customer_id": "cust"}
	createResp, diags := callAPI(ctx, c, pathVPCCreate, createBody)
	if diags.HasError() {
		t.Fatalf("create call failed: %v", diags)
	}

	var createData map[string]interface{}
	if err := json.Unmarshal(createResp.Data, &createData); err != nil {
		t.Fatalf("parse create: %v", err)
	}
	vpcID, _ := createData["vpcId"].(string)
	if vpcID != "vpc-10" {
		t.Fatalf("expected vpc-10, got %q", vpcID)
	}

	// Step 2: pollUntilReady
	pollBody := map[string]interface{}{"vpc_id": vpcID, "customer_id": "cust"}
	if err := pollUntilReady(ctx, c, pathVPCDetail, pollBody, 30*time.Second); err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if atomic.LoadInt32(&pollCalls) < 2 {
		t.Fatalf("expected >= 2 poll calls, got %d", atomic.LoadInt32(&pollCalls))
	}
}

func TestVPCCreatePollFlow_CreationFails(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCCreate, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"vpcId": "vpc-err"}
	})
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "error"}
	})

	c := srv.newClient()
	ctx := context.Background()

	pollBody := map[string]interface{}{"vpc_id": "vpc-err", "customer_id": "cust"}
	err := pollUntilReady(ctx, c, pathVPCDetail, pollBody, 30*time.Second)
	if err == nil {
		t.Fatal("expected error when VPC reaches error state")
	}
}
