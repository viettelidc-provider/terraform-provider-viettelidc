// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*NetworkInterfaceAttachmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*NetworkInterfaceAttachmentResource)(nil)
	_ resource.ResourceWithImportState = (*NetworkInterfaceAttachmentResource)(nil)
)

// NetworkInterfaceAttachmentResource implements `viettelidc_network_interface_attachment`.
type NetworkInterfaceAttachmentResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type NetworkInterfaceAttachmentModel struct {
	ID                 types.String `tfsdk:"id"`
	NetworkInterfaceID types.String `tfsdk:"network_interface_id"`
	InstanceID         types.String `tfsdk:"instance_id"`
	VpcID              types.String `tfsdk:"vpc_id"`
}

func NewNetworkInterfaceAttachmentResource() resource.Resource {
	return &NetworkInterfaceAttachmentResource{}
}

func (r *NetworkInterfaceAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_network_interface_attachment"
}

func (r *NetworkInterfaceAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attaches a ViettelIDC NIC to a VM instance. Composite ID: nic_id/instance_id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Composite ID: <network_interface_id>/<instance_id>.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"network_interface_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"instance_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vpc_id": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *NetworkInterfaceAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *NetworkInterfaceAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NetworkInterfaceAttachmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]interface{}{
		"network_interface_id": plan.NetworkInterfaceID.ValueString(),
		"instance_id":          plan.InstanceID.ValueString(),
		"vpc_id":               vpcID,
		"customer_id":          r.customerID,
	}
	if _, diags := callAPI(ctx, r.client, pathNicAttach, body); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until the NIC detail endpoint confirms attachment to the expected instance.
	nicID := plan.NetworkInterfaceID.ValueString()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		detailBody := map[string]interface{}{
			"network_interface_id": nicID,
			"vpc_id":               vpcID,
			"customer_id":          r.customerID,
		}
		apiResp, pollDiags := callAPI(ctx, r.client, pathNicDetail, detailBody)
		if pollDiags.HasError() {
			if apiResp == nil || !isNotFoundMessage(apiResp.Message) {
				// Hard error — not a transient not-found.
				resp.Diagnostics.Append(pollDiags...)
				return
			}
		} else if apiResp != nil {
			attached, attachedTo, err := readAttachedInstance(apiResp)
			if err == nil && attached && attachedTo == plan.InstanceID.ValueString() {
				break
			}
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError(
				"NIC did not attach",
				fmt.Sprintf("NIC %s did not become attached to instance %s within 2 minutes", nicID, plan.InstanceID.ValueString()),
			)
			return
		}
		time.Sleep(3 * time.Second)
	}

	plan.VpcID = types.StringValue(vpcID)
	plan.ID = types.StringValue(buildAttachmentID(plan.NetworkInterfaceID.ValueString(), plan.InstanceID.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkInterfaceAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkInterfaceAttachmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	nicID, instanceID, err := parseAttachmentID(state.ID.ValueString())
	if err != nil {
		// State corruption: surface and let user re-import.
		resp.Diagnostics.AddError("Invalid attachment id in state", err.Error())
		return
	}
	body := map[string]interface{}{
		"network_interface_id": nicID,
		"vpc_id":               state.VpcID.ValueString(),
		"customer_id":          r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathNicDetail, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}
	attached, attachedTo, err := readAttachedInstance(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode NIC detail", err.Error())
		return
	}
	if !attached || attachedTo != instanceID {
		// Drift: NIC was detached or re-attached elsewhere.
		resp.State.RemoveResource(ctx)
		return
	}
	state.NetworkInterfaceID = types.StringValue(nicID)
	state.InstanceID = types.StringValue(instanceID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op; both attribute changes force replacement.
func (r *NetworkInterfaceAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NetworkInterfaceAttachmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkInterfaceAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkInterfaceAttachmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]interface{}{
		"network_interface_id": state.NetworkInterfaceID.ValueString(),
		"instance_id":          state.InstanceID.ValueString(),
		"vpc_id":               state.VpcID.ValueString(),
		"customer_id":          r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathNicDetach, body)
	if diags.HasError() {
		if apiResp != nil && (isNotFoundMessage(apiResp.Message) || isNotAttachedMessage(apiResp.Message)) {
			return
		}
		resp.Diagnostics.Append(diags...)
	}
}

func (r *NetworkInterfaceAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nicID, instanceID, err := parseAttachmentID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), buildAttachmentID(nicID, instanceID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network_interface_id"), nicID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), instanceID)...)
}

// ---------- Pure helpers ----------

func buildAttachmentID(nicID, instanceID string) string {
	return nicID + "/" + instanceID
}

func parseAttachmentID(id string) (string, string, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("attachment id must be 'nic_id/instance_id', got %q", id)
	}
	return parts[0], parts[1], nil
}

// readAttachedInstance reports whether the NIC is currently attached and to which instance.
func readAttachedInstance(resp *client.APIResponse) (bool, string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return false, "", err
	}
	for _, key := range []string{"attachedInstanceId", "instanceId", "vmId", "vttInstanceId"} {
		if v := asString(data, key); v != "" {
			return true, v, nil
		}
	}
	return false, "", nil
}
