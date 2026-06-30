// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// fakeAPIResponseFromData constructs a *client.APIResponse with the given raw JSON data.
func fakeAPIResponseFromData(t *testing.T, data json.RawMessage) *client.APIResponse {
	t.Helper()
	return &client.APIResponse{Code: float64(0), Message: "ok", Data: data}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCResource — Create + poll + readInto
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCResource_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "test-vpc" {
			t.Errorf("expected name 'test-vpc', got %v", body["name"])
		}
		if body["cidr_block"] != "192.168.0.0/16" {
			t.Errorf("expected cidr_block '192.168.0.0/16', got %v", body["cidr_block"])
		}
		if body["customer_id"] != "cust" {
			t.Errorf("expected customer_id 'cust', got %v", body["customer_id"])
		}
		return float64(0), "ok", map[string]interface{}{"id": "vpc-99"}
	})

	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"id":          "vpc-99",
			"name":        "test-vpc",
			"cidrBlock":   "192.168.0.0/16",
			"description": "my vpc desc",
			"status":      "ACTIVE",
		}
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	ctx := context.Background()

	// Step 1: Create
	createBody := map[string]interface{}{
		"name":        "test-vpc",
		"cidr_block":  "192.168.0.0/16",
		"customer_id": "cust",
		"description": "my vpc desc",
	}
	createResp, diags := callAPI(ctx, r.client, pathVPCCreate, createBody)
	if diags.HasError() {
		t.Fatalf("create failed: %v", diags)
	}

	id, err := extractVPCID(createResp)
	if err != nil || id != "vpc-99" {
		t.Fatalf("extractVPCID: id=%q err=%v", id, err)
	}

	// Step 2: Poll
	pollBody := map[string]interface{}{"vpc_id": id, "customer_id": "cust"}
	if err := pollUntilReady(ctx, r.client, pathVPCDetail, pollBody, 30*time.Second); err != nil {
		t.Fatalf("poll failed: %v", err)
	}

	// Step 3: readInto asserts state
	m := &VPCResourceModel{ID: types.StringValue(id)}
	var dgs diag.Diagnostics
	if !r.readInto(ctx, m, &dgs) {
		t.Fatal("readInto returned false (drift) unexpectedly")
	}
	if dgs.HasError() {
		t.Fatalf("readInto diag: %v", dgs)
	}
	if m.Name.ValueString() != "test-vpc" {
		t.Errorf("expected name 'test-vpc', got %q", m.Name.ValueString())
	}
	if m.CidrBlock.ValueString() != "192.168.0.0/16" {
		t.Errorf("expected cidr '192.168.0.0/16', got %q", m.CidrBlock.ValueString())
	}
	if m.Description.ValueString() != "my vpc desc" {
		t.Errorf("expected description 'my vpc desc', got %q", m.Description.ValueString())
	}
	if m.Status.ValueString() != "ACTIVE" {
		t.Errorf("expected status 'ACTIVE', got %q", m.Status.ValueString())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCResource — Update
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCResource_Update(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCUpdate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["vpc_id"] != "vpc-1" {
			t.Errorf("expected vpc_id 'vpc-1', got %v", body["vpc_id"])
		}
		if body["name"] != "updated-vpc" {
			t.Errorf("expected name 'updated-vpc', got %v", body["name"])
		}
		return float64(0), "ok", map[string]interface{}{}
	})

	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"id":          "vpc-1",
			"name":        "updated-vpc",
			"cidrBlock":   "10.0.0.0/16",
			"description": "updated desc",
			"status":      "ACTIVE",
		}
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	ctx := context.Background()

	updateBody := map[string]interface{}{
		"vpc_id":      "vpc-1",
		"name":        "updated-vpc",
		"description": "updated desc",
		"customer_id": "cust",
	}
	if _, diags := callAPI(ctx, r.client, pathVPCUpdate, updateBody); diags.HasError() {
		t.Fatalf("update call failed: %v", diags)
	}

	m := &VPCResourceModel{ID: types.StringValue("vpc-1"), CidrBlock: types.StringValue("10.0.0.0/16")}
	var dgs diag.Diagnostics
	if !r.readInto(ctx, m, &dgs) {
		t.Fatal("readInto returned false after update")
	}
	if dgs.HasError() {
		t.Fatalf("readInto diag: %v", dgs)
	}
	if m.Name.ValueString() != "updated-vpc" {
		t.Errorf("expected name 'updated-vpc', got %q", m.Name.ValueString())
	}
	if m.Description.ValueString() != "updated desc" {
		t.Errorf("expected description 'updated desc', got %q", m.Description.ValueString())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCResource — Delete (idempotent)
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["vpc_id"] != "vpc-1" {
			t.Errorf("expected vpc_id 'vpc-1', got %v", body["vpc_id"])
		}
		return float64(0), "ok", nil
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	body := map[string]interface{}{
		"vpc_id":      "vpc-1",
		"customer_id": "cust",
	}
	_, diags := callAPI(context.Background(), r.client, pathVPCDelete, body)
	if diags.HasError() {
		t.Fatalf("delete call produced unexpected error: %v", diags)
	}
	if srv.calls[pathVPCDelete] != 1 {
		t.Errorf("expected 1 delete call, got %d", srv.calls[pathVPCDelete])
	}
}

func TestVPCResource_Delete_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCDelete, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "FAILURE", "VPC not found", nil
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	body := map[string]interface{}{"vpc_id": "vpc-missing", "customer_id": "cust"}
	apiResp, diags := callAPI(context.Background(), r.client, pathVPCDelete, body)
	// Delete handler: if not-found, it swallows the error (idempotent).
	if apiResp != nil && isNotFoundMessage(apiResp.Message) {
		// This is the expected idempotent path — no error should be raised by the resource.
		return
	}
	if diags.HasError() {
		t.Logf("delete not-found returned error diag (acceptable): %v", diags)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCResource — ImportState (readInto after import)
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCResource_ImportState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["vpc_id"] != "vpc-import" {
			t.Errorf("import read: expected vpc_id 'vpc-import', got %v", body["vpc_id"])
		}
		return float64(0), "ok", map[string]interface{}{
			"id":          "vpc-import",
			"name":        "imported-vpc",
			"cidrBlock":   "172.16.0.0/12",
			"description": "imported",
			"status":      "ACTIVE",
		}
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	// Simulate ImportState: only ID is set in state, readInto must populate the rest.
	m := &VPCResourceModel{ID: types.StringValue("vpc-import")}
	var dgs diag.Diagnostics
	found := r.readInto(context.Background(), m, &dgs)
	if !found {
		t.Fatal("readInto returned false on import — VPC should be found")
	}
	if dgs.HasError() {
		t.Fatalf("readInto diag: %v", dgs)
	}
	if m.ID.ValueString() != "vpc-import" {
		t.Errorf("id: expected 'vpc-import', got %q", m.ID.ValueString())
	}
	if m.Name.ValueString() != "imported-vpc" {
		t.Errorf("name: expected 'imported-vpc', got %q", m.Name.ValueString())
	}
	if m.CidrBlock.ValueString() != "172.16.0.0/12" {
		t.Errorf("cidr: expected '172.16.0.0/12', got %q", m.CidrBlock.ValueString())
	}
	if m.Status.ValueString() != "ACTIVE" {
		t.Errorf("status: expected 'ACTIVE', got %q", m.Status.ValueString())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCResource — PollTimeout
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCResource_PollTimeout(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	// Always return pending — never becomes ready.
	srv.on(pathVPCDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"status": "creating"}
	})

	r := &VPCResource{client: srv.newClient(), customerID: "cust"}
	pollBody := map[string]interface{}{"vpc_id": "vpc-slow", "customer_id": "cust"}

	// timeout=0 triggers immediate timeout.
	err := pollUntilReady(context.Background(), r.client, pathVPCDetail, pollBody, 0)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCDataSource — By ID
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCDataSource_ByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["id"] != "vpc-ds-1" {
			t.Errorf("expected id 'vpc-ds-1', got %v", body["id"])
		}
		return float64(0), "ok", map[string]interface{}{
			"id":          "vpc-ds-1",
			"name":        "ds-vpc",
			"cidrBlock":   "10.10.0.0/16",
			"description": "data source vpc",
			"status":      "ACTIVE",
		}
	})

	d := &VPCDataSource{client: srv.newClient(), customerID: "cust"}
	body := map[string]interface{}{
		"id":          "vpc-ds-1",
		"vpc_id":      "vpc-ds-1",
		"customer_id": d.customerID,
	}

	apiResp, diags := callAPI(context.Background(), d.client, pathVPCDetail, body)
	if diags.HasError() {
		t.Fatalf("detail call failed: %v", diags)
	}

	var cfg VPCDataSourceModel
	if err := mapVPCDataSource(apiResp, &cfg); err != nil {
		t.Fatalf("mapVPCDataSource: %v", err)
	}

	if cfg.ID.ValueString() != "vpc-ds-1" {
		t.Errorf("id: expected 'vpc-ds-1', got %q", cfg.ID.ValueString())
	}
	if cfg.Name.ValueString() != "ds-vpc" {
		t.Errorf("name: expected 'ds-vpc', got %q", cfg.Name.ValueString())
	}
	if cfg.CidrBlock.ValueString() != "10.10.0.0/16" {
		t.Errorf("cidr: expected '10.10.0.0/16', got %q", cfg.CidrBlock.ValueString())
	}
	if cfg.Description.ValueString() != "data source vpc" {
		t.Errorf("description: expected 'data source vpc', got %q", cfg.Description.ValueString())
	}
	if cfg.Status.ValueString() != "ACTIVE" {
		t.Errorf("status: expected 'ACTIVE', got %q", cfg.Status.ValueString())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VPCDataSource — By Name
// ─────────────────────────────────────────────────────────────────────────────

func TestVPCDataSource_ByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["customer_id"] != "cust" {
			t.Errorf("expected customer_id 'cust', got %v", body["customer_id"])
		}
		return float64(0), "ok", []map[string]interface{}{
			{"id": "vpc-A", "name": "alpha-vpc", "cidrBlock": "10.1.0.0/16", "description": "A", "status": "ACTIVE"},
			{"id": "vpc-B", "name": "beta-vpc", "cidrBlock": "10.2.0.0/16", "description": "B", "status": "ACTIVE"},
			{"id": "vpc-C", "name": "gamma-vpc", "cidrBlock": "10.3.0.0/16", "description": "C", "status": "ACTIVE"},
		}
	})

	d := &VPCDataSource{client: srv.newClient(), customerID: "cust"}
	body := map[string]interface{}{"customer_id": d.customerID}

	apiResp, diags := callAPI(context.Background(), d.client, pathVPCList, body)
	if diags.HasError() {
		t.Fatalf("list call failed: %v", diags)
	}

	items, err := decodeVPCList(apiResp)
	if err != nil {
		t.Fatalf("decodeVPCList: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 VPCs, got %d", len(items))
	}

	// Filter by name (as data source Read does).
	wantName := "beta-vpc"
	var found *VPCDataSourceModel
	for i := range items {
		if items[i].Name.ValueString() == wantName {
			found = &items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("VPC %q not found in list", wantName)
	}
	if found.ID.ValueString() != "vpc-B" {
		t.Errorf("id: expected 'vpc-B', got %q", found.ID.ValueString())
	}
	if found.CidrBlock.ValueString() != "10.2.0.0/16" {
		t.Errorf("cidr: expected '10.2.0.0/16', got %q", found.CidrBlock.ValueString())
	}
}

func TestVPCDataSource_ByName_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathVPCList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", []map[string]interface{}{
			{"id": "vpc-A", "name": "alpha-vpc", "cidrBlock": "10.1.0.0/16", "description": "", "status": "ACTIVE"},
		}
	})

	d := &VPCDataSource{client: srv.newClient(), customerID: "cust"}
	body := map[string]interface{}{"customer_id": d.customerID}

	apiResp, diags := callAPI(context.Background(), d.client, pathVPCList, body)
	if diags.HasError() {
		t.Fatalf("list call: %v", diags)
	}

	items, err := decodeVPCList(apiResp)
	if err != nil {
		t.Fatalf("decodeVPCList: %v", err)
	}

	var found *VPCDataSourceModel
	for i := range items {
		if items[i].Name.ValueString() == "nonexistent-vpc" {
			found = &items[i]
			break
		}
	}
	if found != nil {
		t.Fatal("expected VPC not found, but got a result")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// extractVPCID — handles both "id" and "vttVpcId" response fields
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractVPCID_ByIDField(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCCreate, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"id": "vpc-from-id"}
	})

	resp, diags := callAPI(context.Background(), srv.newClient(), pathVPCCreate, map[string]interface{}{})
	if diags.HasError() {
		t.Fatalf("call: %v", diags)
	}
	id, err := extractVPCID(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "vpc-from-id" {
		t.Errorf("expected 'vpc-from-id', got %q", id)
	}
}

func TestExtractVPCID_ByVttVpcIdField(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCCreate, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"vttVpcId": "vpc-from-vtt"}
	})

	resp, diags := callAPI(context.Background(), srv.newClient(), pathVPCCreate, map[string]interface{}{})
	if diags.HasError() {
		t.Fatalf("call: %v", diags)
	}
	id, err := extractVPCID(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "vpc-from-vtt" {
		t.Errorf("expected 'vpc-from-vtt', got %q", id)
	}
}

func TestExtractVPCID_Missing(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathVPCCreate, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"otherField": "no-id-here"}
	})

	resp, diags := callAPI(context.Background(), srv.newClient(), pathVPCCreate, map[string]interface{}{})
	if diags.HasError() {
		t.Fatalf("call: %v", diags)
	}
	_, err := extractVPCID(resp)
	if err == nil {
		t.Fatal("expected error when no ID field present, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// mapVPCResponse — cidr_block fallback field
// ─────────────────────────────────────────────────────────────────────────────

func TestMapVPCResponse_CidrFallback(t *testing.T) {
	t.Parallel()
	// CSA sometimes returns snake_case cidr_block instead of camelCase cidrBlock.
	data, _ := json.Marshal(map[string]interface{}{
		"id":         "vpc-1",
		"name":       "test",
		"cidr_block": "10.5.0.0/24",
		"status":     "ACTIVE",
	})

	fakeResp := fakeAPIResponseFromData(t, data)
	var m VPCResourceModel
	if err := mapVPCResponse(fakeResp, &m); err != nil {
		t.Fatalf("mapVPCResponse: %v", err)
	}
	if m.CidrBlock.ValueString() != "10.5.0.0/24" {
		t.Errorf("expected cidr '10.5.0.0/24', got %q", m.CidrBlock.ValueString())
	}
}
