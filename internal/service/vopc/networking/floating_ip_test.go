// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFloatingIP_AssociateNICOnly(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	var capturedAssocBody map[string]interface{}
	srv.on(pathFloatingIPDetail, func(b map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"id":                     "5728",
			"vttFloatingId":          "5728",
			"floatingIp":             "103.1.2.3",
			"vttNetworkInterfaceId": "1278302",
			"vpcId":                  "40784",
		}
	})
	srv.on(pathFloatingIPAssociate, func(b map[string]interface{}) (interface{}, string, interface{}) {
		capturedAssocBody = b
		return float64(0), "success", map[string]interface{}{"status": true}
	})

	r := &FloatingIPResource{
		client:       srv.newClient(),
		customerID:   "238250",
		defaultVpcID: "40784",
	}

	plan := FloatingIPResourceModel{
		ID:                 types.StringValue("5728"),
		NetworkInterfaceID: types.StringValue("1278302"),
		InstanceID:         types.StringNull(),
		VpcID:              types.StringValue("40784"),
	}

	// Step 1 & 2 logic from Create:
	hasNIC := !plan.NetworkInterfaceID.IsNull() && plan.NetworkInterfaceID.ValueString() != ""
	hasInstance := !plan.InstanceID.IsNull() && plan.InstanceID.ValueString() != ""

	if !hasNIC && !hasInstance {
		t.Fatal("expected hasNIC to be true")
	}

	assocBody := map[string]interface{}{
		"floating_ip_id":        "5728",
		"vttFloatingId":         parseInt("5728"),
		"vpc_id":                "40784",
		"vpcId":                 parseInt("40784"),
		"customer_id":           r.customerID,
		"customerId":            parseInt(r.customerID),
	}
	if hasNIC {
		assocBody["network_interface_id"] = plan.NetworkInterfaceID.ValueString()
		assocBody["vttNetworkInterfaceId"] = plan.NetworkInterfaceID.ValueString()
	}
	if hasInstance {
		assocBody["instance_id"] = plan.InstanceID.ValueString()
		assocBody["vttVmId"] = parseInt(plan.InstanceID.ValueString())
	}

	apiResp, diags := callAPI(context.Background(), r.client, pathFloatingIPAssociate, assocBody)
	if diags.HasError() {
		t.Fatalf("callAPI associate failed: %v", diags)
	}
	if apiResp == nil || !apiResp.IsSuccess() {
		t.Fatalf("expected success, got %v", apiResp)
	}

	if capturedAssocBody == nil {
		t.Fatal("pathFloatingIPAssociate was not called")
	}
	if capturedAssocBody["vttNetworkInterfaceId"] != "1278302" {
		t.Errorf("vttNetworkInterfaceId = %v, want 1278302", capturedAssocBody["vttNetworkInterfaceId"])
	}
	if capturedAssocBody["vttFloatingId"] != float64(5728) && capturedAssocBody["vttFloatingId"] != int64(5728) && capturedAssocBody["vttFloatingId"] != 5728 {
		t.Errorf("vttFloatingId = %v, want 5728", capturedAssocBody["vttFloatingId"])
	}
	if _, ok := capturedAssocBody["instance_id"]; ok {
		t.Error("instance_id should not be present in assocBody when InstanceID is null")
	}
	if _, ok := capturedAssocBody["vttVmId"]; ok {
		t.Error("vttVmId should not be present in assocBody when InstanceID is null")
	}
}

func TestFloatingIP_DisassociateNICOnly(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	var capturedDisassocBody map[string]interface{}
	srv.on(pathFloatingIPDisassociate, func(b map[string]interface{}) (interface{}, string, interface{}) {
		capturedDisassocBody = b
		return float64(0), "success", map[string]interface{}{"status": true}
	})

	r := &FloatingIPResource{
		client:       srv.newClient(),
		customerID:   "238250",
		defaultVpcID: "40784",
	}

	state := FloatingIPResourceModel{
		ID:                 types.StringValue("5728"),
		NetworkInterfaceID: types.StringValue("1278302"),
		InstanceID:         types.StringNull(),
		VpcID:              types.StringValue("40784"),
	}

	hasNIC := !state.NetworkInterfaceID.IsNull() && state.NetworkInterfaceID.ValueString() != ""
	hasInstance := !state.InstanceID.IsNull() && state.InstanceID.ValueString() != ""

	if !hasNIC && !hasInstance {
		t.Fatal("expected hasNIC to be true")
	}

	body := map[string]interface{}{
		"floating_ip_id": state.ID.ValueString(),
		"vttFloatingId":  parseInt(state.ID.ValueString()),
		"vpc_id":         state.VpcID.ValueString(),
		"vpcId":          parseInt(state.VpcID.ValueString()),
		"customer_id":    r.customerID,
		"customerId":     parseInt(r.customerID),
	}
	_, diags := callAPI(context.Background(), r.client, pathFloatingIPDisassociate, body)
	if diags.HasError() {
		t.Fatalf("callAPI disassociate failed: %v", diags)
	}

	if capturedDisassocBody == nil {
		t.Fatal("pathFloatingIPDisassociate was not called")
	}
	if capturedDisassocBody["vttFloatingId"] != float64(5728) && capturedDisassocBody["vttFloatingId"] != int64(5728) && capturedDisassocBody["vttFloatingId"] != 5728 {
		t.Errorf("vttFloatingId = %v, want 5728", capturedDisassocBody["vttFloatingId"])
	}
}

func TestNetworkInterface_UnknownDescription_SetsNull(t *testing.T) {
	t.Parallel()
	m := &NetworkInterfaceResourceModel{
		ID:          types.StringValue("1278302"),
		Description: types.StringUnknown(),
	}

	dataWithoutDescription := map[string]interface{}{
		"id":           "1278302",
		"name":         "test-nic",
		"vttSubnetId":  "10416",
		"ipAssignType": "auto",
		"primaryIp4":   "10.0.2.134",
		"status":       "success",
	}

	fillNicFromList(dataWithoutDescription, m)

	if m.Description.IsUnknown() {
		t.Error("Description must not remain unknown after fillNicFromList")
	}
	if !m.Description.IsNull() {
		t.Errorf("Description should be null when not returned by API, got %v", m.Description)
	}
}
