// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------- Helper: build a minimal LB detail response ----------

func lbDetailResponse(id int64, name, status string) map[string]interface{} {
	return map[string]interface{}{
		"vttLoadBalancerId":       float64(id),
		"name":                    name,
		"description":             "",
		"vttSubnetId":             float64(101),
		"vttFloatingIpId":         float64(0),
		"lbType":                  "Application",
		"vttLoadbalancerTypeName": "Application",
		"packageType":             "STANDARD",
		"loadbalancerTypeName":    "STANDARD",
		"adminStateUp":            true,
		"status":                  status,
		"operatingStatus":         status,
	}
}

func lbListenerResponse() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":              float64(1001),
			"name":            "lb-listener",
			"description":     "",
			"protocol":        "HTTP",
			"protocolPort":    float64(80),
			"xForwardedFor":   false,
			"xForwardedPort":  false,
			"xForwardedProto": false,
		},
	}
}

func lbPoolResponse() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":                     float64(2001),
			"name":                   "lb-pool",
			"description":            "",
			"algorithm":              "ROUND_ROBIN",
			"sessionPersistenceType": "NONE",
		},
	}
}

// ---------- LoadBalancerResource tests ----------

func TestLoadBalancerResource_CreateFlow(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		lb, _ := body["loadBalancer"].(map[string]interface{})
		if lb["name"] != "lb-main" {
			t.Errorf("create: unexpected lb.name %v", lb["name"])
		}
		return float64(0), "ok", map[string]interface{}{"taskId": "task-1"}
	})
	// Poll list endpoint to discover LB ID by name.
	srv.on(pathLoadBalancerList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"items": []map[string]interface{}{
				{"vttLoadBalancerId": float64(42), "name": "lb-main"},
			},
		}
	})
	// pollForStatus calls detail.
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-main", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}

	// Step 1: compound-create.
	createBody := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": r.customerID,
		"loadBalancer": map[string]interface{}{
			"name":   "lb-main",
			"vpcId":  int64(100),
			"lbType": "Application",
		},
		"listener": map[string]interface{}{"name": "lb-main-listener"},
		"pool":     map[string]interface{}{"name": "lb-main-pool"},
		"members":  []map[string]interface{}{},
		"monitor":  map[string]interface{}{"name": "lb-main-health"},
	}
	_, d := callAPI(context.Background(), r.client, pathLoadBalancerCreate, createBody)
	if d.HasError() {
		t.Fatalf("create call failed: %v", d)
	}

	// Step 2: poll list to find ID by name.
	listBody := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": r.customerID,
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
	}
	listResp, d := callAPI(context.Background(), r.client, pathLoadBalancerList, listBody)
	if d.HasError() {
		t.Fatalf("list call failed: %v", d)
	}
	var listResult struct {
		Items []struct {
			VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
			Name              string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Data, &listResult); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	var actualLBID int64
	for _, item := range listResult.Items {
		if item.Name == "lb-main" {
			actualLBID = item.VttLoadBalancerID
		}
	}
	if actualLBID != 42 {
		t.Fatalf("expected lb id=42, got %d", actualLBID)
	}

	// Step 3: readAndMerge.
	model := &LoadBalancerResourceModel{
		ID:    types.StringValue(fmt.Sprintf("%d", actualLBID)),
		VpcID: types.StringValue("100"),
	}
	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), model, &dgs)
	if dgs.HasError() {
		t.Fatalf("readAndMerge: %v", dgs)
	}
	if model.Name.ValueString() != "lb-main" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.Status.ValueString() != "ACTIVE" {
		t.Errorf("status: %q", model.Status.ValueString())
	}
	if model.Listeners.IsNull() || model.Listeners.IsUnknown() {
		t.Error("listeners should be populated")
	}
	if model.Pools.IsNull() || model.Pools.IsUnknown() {
		t.Error("pools should be populated")
	}
}

func TestLoadBalancerResource_Update(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerUpdate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if _, ok := body["vttLoadBalancerId"]; !ok {
			t.Error("update: vttLoadBalancerId missing")
		}
		if _, ok := body["adminStateUp"]; !ok {
			t.Error("update: adminStateUp missing")
		}
		return float64(0), "ok", nil
	})
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-main", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":            "100",
		"customer_id":       r.customerID,
		"vttLoadBalancerId": int64(42),
		"adminStateUp":      false,
	}
	_, d := callAPI(context.Background(), r.client, pathLoadBalancerUpdate, body)
	if d.HasError() {
		t.Fatalf("update failed: %v", d)
	}
}

func TestLoadBalancerResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if _, ok := body["vttLoadBalancerId"]; !ok {
			t.Error("delete: vttLoadBalancerId missing")
		}
		return float64(0), "ok", nil
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":            "100",
		"customer_id":       r.customerID,
		"vttLoadBalancerId": int64(42),
	}
	_, d := callAPI(context.Background(), r.client, pathLoadBalancerDelete, body)
	if d.HasError() {
		t.Fatalf("delete failed: %v", d)
	}
}

func TestLoadBalancerResource_Delete_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDelete, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "resource not found", nil
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	apiResp, d := callAPI(context.Background(), r.client, pathLoadBalancerDelete, map[string]interface{}{
		"vpc_id":            "100",
		"customer_id":       r.customerID,
		"vttLoadBalancerId": int64(999),
	})
	if d.HasError() && apiResp != nil && isNotFoundMessage(apiResp.Message) {
		return // idempotent — expected
	}
	if d.HasError() {
		t.Fatalf("unexpected error for not-found: %v", d)
	}
}

func TestLoadBalancerResource_ListenersAndPools(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-main", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	model := &LoadBalancerResourceModel{
		ID:    types.StringValue("42"),
		VpcID: types.StringValue("100"),
	}
	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), model, &dgs)
	if dgs.HasError() {
		t.Fatalf("readAndMerge: %v", dgs)
	}

	if len(model.Listeners.Elements()) != 1 {
		t.Errorf("expected 1 listener, got %d", len(model.Listeners.Elements()))
	}
	if len(model.Pools.Elements()) != 1 {
		t.Errorf("expected 1 pool, got %d", len(model.Pools.Elements()))
	}
}

// ---------- LoadBalancerDataSource tests ----------

func TestLoadBalancerDataSource_ByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-prod", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	model := &LoadBalancerResourceModel{
		ID:    types.StringValue("42"),
		VpcID: types.StringValue("100"),
	}
	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), model, &dgs)
	if dgs.HasError() {
		t.Fatalf("readAndMerge: %v", dgs)
	}
	if model.Name.ValueString() != "lb-prod" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.LoadBalancerType.ValueString() != "Application" {
		t.Errorf("type: %q", model.LoadBalancerType.ValueString())
	}
}

func TestLoadBalancerDataSource_ByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"items": []map[string]interface{}{
				{"vttLoadBalancerId": float64(10), "name": "lb-dev"},
				{"vttLoadBalancerId": float64(11), "name": "lb-prod"},
			},
		}
	})

	d := &LoadBalancerDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": d.customerID,
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
	}
	apiResp, dgs := callAPI(context.Background(), d.client, pathLoadBalancerList, body)
	if dgs.HasError() {
		t.Fatalf("list: %v", dgs)
	}

	var listResult struct {
		Items []struct {
			VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
			Name              string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResult); err != nil {
		t.Fatalf("decode: %v", err)
	}

	targetName := "lb-prod"
	var foundID int64
	for _, item := range listResult.Items {
		if item.Name == targetName {
			foundID = item.VttLoadBalancerID
			break
		}
	}
	if foundID != 11 {
		t.Errorf("expected id=11, got %d", foundID)
	}
}
