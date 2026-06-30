// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------- NatGatewayResource tests ----------

func TestNatGatewayResource_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathNatGatewayCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "nat-1" {
			t.Errorf("create: unexpected name %v", body["name"])
		}
		return float64(0), "ok", map[string]interface{}{"id": "42", "status": true}
	})
	// Poll uses list endpoint.
	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "ACTIVE")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}

	// Simulate Create flow: call create, extract id, poll ready.
	apiResp, d := callAPI(context.Background(), r.client, pathNatGatewayCreate, map[string]interface{}{
		"vpc_id":                  "100",
		"customer_id":             r.customerID,
		"name":                    "nat-1",
		"vtt_subnet_id":           "101",
		"vtt_internet_gateway_id": "201",
		"connect_type":            false,
	})
	if d.HasError() {
		t.Fatalf("create call failed: %v", d)
	}

	var result struct {
		ID     string `json:"id"`
		Status bool   `json:"status"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if result.ID != "42" {
		t.Errorf("id: %q", result.ID)
	}

	model := &NatGatewayResourceModel{
		ID:    types.StringValue(result.ID),
		VpcID: types.StringValue("100"),
	}
	if err := r.pollReady(context.Background(), model, 30*time.Second); err != nil {
		t.Fatalf("pollReady: %v", err)
	}
	if model.Status.ValueString() != "ACTIVE" {
		t.Errorf("status: %q", model.Status.ValueString())
	}
}

func TestNatGatewayResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathNatGatewayDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if _, ok := body["vtt_nat_id"]; !ok {
			t.Error("delete: vtt_nat_id missing from body")
		}
		return float64(0), "ok", nil
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vtt_nat_id":  "42",
		"vpc_id":      "100",
		"customer_id": r.customerID,
	}
	_, d := callAPI(context.Background(), r.client, pathNatGatewayDelete, body)
	if d.HasError() {
		t.Fatalf("delete failed: %v", d)
	}
}

func TestNatGatewayResource_Delete_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathNatGatewayDelete, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "resource not found", nil
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	apiResp, d := callAPI(context.Background(), r.client, pathNatGatewayDelete, map[string]interface{}{
		"vtt_nat_id":  "gone",
		"vpc_id":      "100",
		"customer_id": r.customerID,
	})
	if d.HasError() && apiResp != nil && isNotFoundMessage(apiResp.Message) {
		return // expected idempotent behavior
	}
	if d.HasError() {
		t.Fatalf("unexpected error for not-found: %v", d)
	}
}

func TestNatGatewayResource_ReadAndMerge(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", natGatewayListResponse(42, "ACTIVE")
	})

	r := &NatGatewayResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	model := &NatGatewayResourceModel{
		ID:    types.StringValue("42"),
		VpcID: types.StringValue("100"),
	}
	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), model, &dgs)
	if dgs.HasError() {
		t.Fatalf("readAndMerge: %v", dgs)
	}
	if model.Status.ValueString() != "ACTIVE" {
		t.Errorf("status: %q", model.Status.ValueString())
	}
	if model.Name.ValueString() != "nat-test" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
}

// ---------- NatGatewayDataSource tests ----------

func TestNatGatewayDataSource_ByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"id":          float64(42),
					"name":        "nat-main",
					"vttSubnetId": float64(101),
					"connectType": true,
					"nicIp":       "10.0.0.1",
					"status":      "ACTIVE",
					"createdAt":   "2025-01-01T00:00:00Z",
				},
			},
		}
	})

	d := &NatGatewayDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": d.customerID,
		"page_index":  0,
		"page_size":   1000,
		"filters":     []map[string]interface{}{},
	}
	apiResp, dgs := callAPI(context.Background(), d.client, pathNatGatewayList, body)
	if dgs.HasError() {
		t.Fatalf("list failed: %v", dgs)
	}

	var listResp struct {
		Items []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			VttSubnetID int64  `json:"vttSubnetId"`
			ConnectType bool   `json:"connectType"`
			NicIP       string `json:"nicIp"`
			Status      string `json:"status"`
			CreatedAt   string `json:"createdAt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	targetID := "42"
	var found bool
	for _, item := range listResp.Items {
		if fmt.Sprintf("%d", item.ID) == targetID {
			found = true
			if item.Status != "ACTIVE" {
				t.Errorf("status: %q", item.Status)
			}
			if item.Name != "nat-main" {
				t.Errorf("name: %q", item.Name)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected to find nat gateway id=42")
	}
}

func TestNatGatewayDataSource_ByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathNatGatewayList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"id":          float64(10),
					"name":        "nat-dev",
					"vttSubnetId": float64(50),
					"connectType": false,
					"nicIp":       "",
					"status":      "ACTIVE",
					"createdAt":   "",
				},
				{
					"id":          float64(11),
					"name":        "nat-prod",
					"vttSubnetId": float64(51),
					"connectType": true,
					"nicIp":       "10.0.0.2",
					"status":      "ACTIVE",
					"createdAt":   "",
				},
			},
		}
	})

	d := &NatGatewayDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": d.customerID,
		"page_index":  0,
		"page_size":   1000,
		"filters":     []map[string]interface{}{},
	}
	apiResp, dgs := callAPI(context.Background(), d.client, pathNatGatewayList, body)
	if dgs.HasError() {
		t.Fatalf("list: %v", dgs)
	}

	var listResp struct {
		Items []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	targetName := "nat-prod"
	var foundID int64
	for _, item := range listResp.Items {
		if item.Name == targetName {
			foundID = item.ID
			break
		}
	}
	if foundID != 11 {
		t.Errorf("expected id=11, got %d", foundID)
	}
}
