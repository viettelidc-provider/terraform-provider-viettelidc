// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vpc

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------- LaunchTemplateDataSource tests (Story 9.3) ----------

func TestUnit_LaunchTemplateDataSource_ByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["id"] == nil {
			t.Error("id missing from detail request")
		}
		return float64(0), "Success", map[string]interface{}{
			"id":          "lt-abc123",
			"name":        "web-template",
			"description": "my desc",
			"memorySize":  float64(4096),
			"cpuSize":     float64(2),
			"vmId":        "vm-abc",
		}
	})

	r := &LaunchTemplateDataSource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	body := map[string]interface{}{
		"id":          "lt-abc123",
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(context.Background(), r.client, pathLaunchTemplateDetail, body)
	if diags.HasError() {
		t.Fatalf("detail call failed: %v", diags)
	}

	model := &LaunchTemplateDataSourceModel{
		ID:    types.StringValue("lt-abc123"),
		VpcID: types.StringValue("36961"),
	}
	if err := mapLaunchTemplateToDataSource(apiResp, model, "36961"); err != nil {
		t.Fatalf("mapLaunchTemplateToDataSource: %v", err)
	}

	if model.Name.ValueString() != "web-template" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.Description.ValueString() != "my desc" {
		t.Errorf("description: %q", model.Description.ValueString())
	}
	if model.MemorySize.ValueInt64() != 4096 {
		t.Errorf("memory_size: %d", model.MemorySize.ValueInt64())
	}
	if model.CpuSize.ValueInt64() != 2 {
		t.Errorf("cpu_size: %d", model.CpuSize.ValueInt64())
	}
}

func TestUnit_LaunchTemplateDataSource_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "Launch Template not found", nil
	})

	r := &LaunchTemplateDataSource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	body := map[string]interface{}{
		"id":          "lt-gone",
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(context.Background(), r.client, pathLaunchTemplateDetail, body)
	if !diags.HasError() {
		t.Fatal("expected error diag for not-found API response")
	}
	if apiResp == nil {
		t.Fatal("expected apiResp even on error")
	}
	if !isNotFoundMessage(apiResp.Message) {
		t.Errorf("expected not-found message, got: %q", apiResp.Message)
	}
}

func TestUnit_LaunchTemplateDataSource_DefaultVpcID(t *testing.T) {
	t.Parallel()
	// When vpc_id is empty, provider defaultVpcID should be used.
	r := &LaunchTemplateDataSource{client: nil, customerID: "c", defaultVpcID: "default-vpc"}

	got, diags := resolveVpcID("", r.defaultVpcID)
	if diags.HasError() {
		t.Fatalf("unexpected diag: %v", diags)
	}
	if got != "default-vpc" {
		t.Errorf("got %q want default-vpc", got)
	}
}

func TestUnit_LaunchTemplateDataSource_Schema(t *testing.T) {
	t.Parallel()
	r := &LaunchTemplateDataSource{}
	var sreq datasource.SchemaRequest
	var sresp datasource.SchemaResponse
	r.Schema(context.Background(), sreq, &sresp)

	for _, attrName := range []string{"id", "name", "description", "memory_size", "cpu_size", "vpc_id"} {
		if _, ok := sresp.Schema.Attributes[attrName]; !ok {
			t.Errorf("attribute %q not found in LT data source schema", attrName)
		}
	}
}

// ---------- LaunchTemplatesDataSource tests (Story 9.3) ----------

func TestUnit_LaunchTemplatesDataSource_All(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateListAll, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{
			{"id": "lt-1", "name": "template-1", "memorySize": float64(4096), "cpuSize": float64(2), "description": ""},
			{"id": "lt-2", "name": "template-2", "memorySize": float64(8192), "cpuSize": float64(4), "description": ""},
		}
	})

	r := &LaunchTemplatesDataSource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	body := map[string]interface{}{
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(context.Background(), r.client, pathLaunchTemplateListAll, body)
	if diags.HasError() {
		t.Fatalf("list-all call failed: %v", diags)
	}

	items, err := decodeLaunchTemplateList(apiResp)
	if err != nil {
		t.Fatalf("decodeLaunchTemplateList: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if asIDString(items[0], "id") != "lt-1" {
		t.Errorf("item[0] id: %v", items[0]["id"])
	}
	if asString(items[1], "name") != "template-2" {
		t.Errorf("item[1] name: %v", items[1]["name"])
	}
}

func TestUnit_LaunchTemplatesDataSource_Empty(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateListAll, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{}
	})

	r := &LaunchTemplatesDataSource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	body := map[string]interface{}{
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(context.Background(), r.client, pathLaunchTemplateListAll, body)
	if diags.HasError() {
		t.Fatalf("list-all failed: %v", diags)
	}

	items, err := decodeLaunchTemplateList(apiResp)
	if err != nil {
		t.Fatalf("decodeLaunchTemplateList: %v", err)
	}
	// Empty list is NOT an error — items should be nil or empty slice.
	if len(items) != 0 {
		t.Fatalf("expected empty list, got %d items", len(items))
	}
}

func TestUnit_LaunchTemplatesDataSource_Int64Fields(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateListAll, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{
			{"id": "lt-1", "name": "tpl", "memorySize": float64(4096), "cpuSize": float64(2), "description": ""},
		}
	})

	r := &LaunchTemplatesDataSource{client: srv.newClient(), customerID: "c", defaultVpcID: "v"}
	body := map[string]interface{}{"vpc_id": "v", "customer_id": "c"}
	apiResp, _ := callAPI(context.Background(), r.client, pathLaunchTemplateListAll, body)
	items, _ := decodeLaunchTemplateList(apiResp)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	mem := asInt64(items[0], "memorySize")
	cpu := asInt64(items[0], "cpuSize")
	if mem != 4096 {
		t.Errorf("memory_size: got %d want 4096", mem)
	}
	if cpu != 2 {
		t.Errorf("cpu_size: got %d want 2", cpu)
	}
}
