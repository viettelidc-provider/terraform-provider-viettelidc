// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vpc

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------- LaunchTemplateResource unit tests ----------

func TestUnit_LaunchTemplateResource_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "web-template" {
			t.Errorf("create: unexpected name %v", body["name"])
		}
		return float64(0), "Success", map[string]interface{}{
			"id":          "lt-abc123",
			"name":        "web-template",
			"memorySize":  float64(4096),
			"cpuSize":     float64(2),
			"vmId":        "vm-abc",
			"description": "",
		}
	})

	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	apiResp, d := callAPI(context.Background(), r.client, pathLaunchTemplateCreate, map[string]interface{}{
		"name":        "web-template",
		"vm_id":       "vm-abc",
		"memory_size": int64(4096),
		"cpu_size":    int64(2),
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	})
	if d.HasError() {
		t.Fatalf("create call failed: %v", d)
	}

	id, err := extractLaunchTemplateID(apiResp)
	if err != nil {
		t.Fatalf("extractLaunchTemplateID: %v", err)
	}
	if id != "lt-abc123" {
		t.Errorf("id: got %q want lt-abc123", id)
	}
}

func TestUnit_LaunchTemplateResource_Read(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", map[string]interface{}{
			"id":          "lt-abc123",
			"name":        "web-template",
			"memorySize":  float64(4096),
			"cpuSize":     float64(2),
			"vmId":        "vm-abc",
			"description": "my desc",
		}
	})

	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}
	model := &LaunchTemplateResourceModel{
		ID:    types.StringValue("lt-abc123"),
		VpcID: types.StringValue("36961"),
	}

	var diags diag.Diagnostics
	found := r.readInto(context.Background(), model, &diags)
	if diags.HasError() {
		t.Fatalf("readInto error: %v", diags)
	}
	if !found {
		t.Fatal("readInto returned false (not found)")
	}
	if model.Name.ValueString() != "web-template" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.MemorySize.ValueInt64() != 4096 {
		t.Errorf("memory_size: %d", model.MemorySize.ValueInt64())
	}
	if model.CpuSize.ValueInt64() != 2 {
		t.Errorf("cpu_size: %d", model.CpuSize.ValueInt64())
	}
	if model.Description.ValueString() != "my desc" {
		t.Errorf("description: %q", model.Description.ValueString())
	}
}

func TestUnit_LaunchTemplateResource_Read_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "Launch template not found", nil
	})

	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}
	model := &LaunchTemplateResourceModel{
		ID:    types.StringValue("lt-gone"),
		VpcID: types.StringValue("36961"),
	}

	var diags diag.Diagnostics
	found := r.readInto(context.Background(), model, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if found {
		t.Error("readInto should return false for not-found")
	}
}

func TestUnit_LaunchTemplateResource_ForceNew(t *testing.T) {
	t.Parallel()
	r := &LaunchTemplateResource{}
	var sreq resource.SchemaRequest
	var sresp resource.SchemaResponse
	r.Schema(context.Background(), sreq, &sresp)

	forceNewAttrs := []string{"name", "vm_id", "memory_size", "cpu_size", "vpc_id"}
	for _, attrName := range forceNewAttrs {
		_, ok := sresp.Schema.Attributes[attrName]
		if !ok {
			t.Errorf("attribute %q not found in schema", attrName)
		}
	}
	// Verify description is also present
	if _, ok := sresp.Schema.Attributes["description"]; !ok {
		t.Error("attribute description not found in schema")
	}
	// Verify id is present and computed
	if _, ok := sresp.Schema.Attributes["id"]; !ok {
		t.Error("attribute id not found in schema")
	}
}

func TestUnit_LaunchTemplateResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["id"] == nil {
			t.Error("delete: id missing from body")
		}
		return float64(0), "Success", nil
	})

	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	_, d := callAPI(context.Background(), r.client, pathLaunchTemplateDelete, map[string]interface{}{
		"id":          "lt-abc123",
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	})
	if d.HasError() {
		t.Fatalf("delete call failed: %v", d)
	}
}

func TestUnit_LaunchTemplateResource_Delete_InUse(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateDelete, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "Launch template is in use by autoscale group", nil
	})

	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	apiResp, d := callAPI(context.Background(), r.client, pathLaunchTemplateDelete, map[string]interface{}{
		"id":          "lt-abc123",
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	})
	if !d.HasError() {
		t.Fatal("expected error diag for API non-success")
	}
	if apiResp == nil {
		t.Fatal("expected apiResp to be returned even on error")
	}
	if !isInUseMessage(apiResp.Message) {
		t.Errorf("expected in-use message, got: %q", apiResp.Message)
	}
}

func TestUnit_LaunchTemplateResource_Import(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLaunchTemplateDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", map[string]interface{}{
			"id":          "lt-abc123",
			"name":        "imported-template",
			"memorySize":  float64(8192),
			"cpuSize":     float64(4),
			"vmId":        "vm-xyz",
			"description": "imported desc",
		}
	})

	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}
	model := &LaunchTemplateResourceModel{
		ID:    types.StringValue("lt-abc123"),
		VpcID: types.StringValue("36961"),
	}

	var diags diag.Diagnostics
	found := r.readInto(context.Background(), model, &diags)
	if diags.HasError() {
		t.Fatalf("readInto for import: %v", diags)
	}
	if !found {
		t.Fatal("readInto should return true for import")
	}
	if model.Name.ValueString() != "imported-template" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.MemorySize.ValueInt64() != 8192 {
		t.Errorf("memory_size: %d", model.MemorySize.ValueInt64())
	}
}

// ---------- Helper tests ----------

func TestUnit_ResolveVpcID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, plan, def, want string
		wantErr               bool
	}{
		{"plan wins", "plan-vpc", "default-vpc", "plan-vpc", false},
		{"falls back to default", "", "default-vpc", "default-vpc", false},
		{"both empty errors", "", "", "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, diags := resolveVpcID(tc.plan, tc.def)
			if tc.wantErr {
				if !diags.HasError() {
					t.Fatal("expected error diag")
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("unexpected diag: %v", diags)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestUnit_IsNotFoundMessage(t *testing.T) {
	t.Parallel()
	yes := []string{"resource not found", "template does not exist", "no such template"}
	no := []string{"forbidden", "internal error", ""}
	for _, m := range yes {
		if !isNotFoundMessage(m) {
			t.Errorf("expected true for %q", m)
		}
	}
	for _, m := range no {
		if isNotFoundMessage(m) {
			t.Errorf("expected false for %q", m)
		}
	}
}

func TestUnit_IsInUseMessage(t *testing.T) {
	t.Parallel()
	yes := []string{"template is in use", "resource being used", "still used by group"}
	no := []string{"not found", "forbidden", ""}
	for _, m := range yes {
		if !isInUseMessage(m) {
			t.Errorf("expected true for %q", m)
		}
	}
	for _, m := range no {
		if isInUseMessage(m) {
			t.Errorf("expected false for %q", m)
		}
	}
}
