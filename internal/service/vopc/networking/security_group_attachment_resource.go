// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	_ resource.Resource                = (*SecurityGroupAttachmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*SecurityGroupAttachmentResource)(nil)
	_ resource.ResourceWithImportState = (*SecurityGroupAttachmentResource)(nil)
)

// SecurityGroupAttachmentResource implements `viettelidc_ovpc_security_group_attachment`.
type SecurityGroupAttachmentResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// SecurityGroupAttachmentModel mirrors the resource schema.
type SecurityGroupAttachmentModel struct {
	ID              types.String `tfsdk:"id"`
	InstanceID      types.String `tfsdk:"instance_id"`
	SecurityGroupID types.String `tfsdk:"security_group_id"`
	VpcID           types.String `tfsdk:"vpc_id"`
}

func NewSecurityGroupAttachmentResource() resource.Resource {
	return &SecurityGroupAttachmentResource{}
}

func (r *SecurityGroupAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_security_group_attachment"
}

func (r *SecurityGroupAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a resource to attach a Security Group to an OVPC instance.\n\n" +
			"~> **WARNING:** Do not use this resource if you are already managing `security_group_ids` inside the `viettelidc_ovpc_instance` resource, " +
			"as they will conflict and cause Terraform state drift.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"instance_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the instance to attach the security group to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"security_group_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the security group to attach.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *SecurityGroupAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *SecurityGroupAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecurityGroupAttachmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	instanceID := plan.InstanceID.ValueString()
	sgID := plan.SecurityGroupID.ValueString()
	instanceIDInt, err := strconv.ParseInt(instanceID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Instance ID", fmt.Sprintf("instance_id %q is not a valid integer: %s", instanceID, err))
		return
	}

	currentSGs, err := r.getCurrentSGs(ctx, instanceID, vpcID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read current Security Groups", err.Error())
		return
	}

	// Check if already attached
	alreadyAttached := false
	for _, sg := range currentSGs {
		if sg == sgID {
			alreadyAttached = true
			break
		}
	}

	if !alreadyAttached {
		// Build payload
		var sgPayload []map[string]interface{}
		// Kept SGs
		for _, sg := range currentSGs {
			sgPayload = append(sgPayload, map[string]interface{}{
				"id":                 sg,
				"vttVmId":            instanceIDInt,
				"vttSecurityGroupId": sg,
			})
		}
		// Added SG
		sgPayload = append(sgPayload, map[string]interface{}{
			"id":                 sgID,
			"type":               "attach",
			"vttVmId":            instanceIDInt,
			"vttSecurityGroupId": sgID,
		})

		vpcIDInt, _ := strconv.ParseInt(vpcID, 10, 64)
		customerIDInt, _ := strconv.ParseInt(r.customerID, 10, 64)

		sgUpdateBody := map[string]interface{}{
			"vttVmId":        instanceIDInt,
			"securityGroups": sgPayload,
			"vpcId":          vpcIDInt,
			"customerId":     customerIDInt,
		}

		if _, diags := callAPI(ctx, r.client, pathSGVmUpdate, sgUpdateBody); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s_%s", instanceID, sgID))
	plan.VpcID = types.StringValue(vpcID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SecurityGroupAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecurityGroupAttachmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	instanceID := state.InstanceID.ValueString()
	sgID := state.SecurityGroupID.ValueString()

	currentSGs, err := r.getCurrentSGs(ctx, instanceID, vpcID)
	if err != nil {
		// If VM is gone, remove resource from state
		if err.Error() == "not_found" {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read current Security Groups", err.Error())
		return
	}

	isAttached := false
	for _, sg := range currentSGs {
		if sg == sgID {
			isAttached = true
			break
		}
	}

	if !isAttached {
		// Drift detected, attachment was removed out-of-band
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SecurityGroupAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Unsupported Operation",
		"Updating a security group attachment is not supported. All attributes force replacement.",
	)
}

func (r *SecurityGroupAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecurityGroupAttachmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	instanceID := state.InstanceID.ValueString()
	sgID := state.SecurityGroupID.ValueString()

	instanceIDInt, err := strconv.ParseInt(instanceID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Instance ID", fmt.Sprintf("instance_id %q is not a valid integer: %s", instanceID, err))
		return
	}

	currentSGs, err := r.getCurrentSGs(ctx, instanceID, vpcID)
	if err != nil {
		if err.Error() == "not_found" {
			// VM already gone, nothing to detach
			return
		}
		resp.Diagnostics.AddError("Failed to read current Security Groups", err.Error())
		return
	}

	isAttached := false
	for _, sg := range currentSGs {
		if sg == sgID {
			isAttached = true
			break
		}
	}

	if isAttached {
		// Build payload
		var sgPayload []map[string]interface{}
		for _, sg := range currentSGs {
			if sg == sgID {
				// Detached SG
				sgPayload = append(sgPayload, map[string]interface{}{
					"id":                 sgID,
					"type":               "detach",
					"vttVmId":            instanceIDInt,
					"vttSecurityGroupId": sgID,
				})
			} else {
				// Kept SGs
				sgPayload = append(sgPayload, map[string]interface{}{
					"id":                 sg,
					"vttVmId":            instanceIDInt,
					"vttSecurityGroupId": sg,
				})
			}
		}

		vpcIDInt, _ := strconv.ParseInt(vpcID, 10, 64)
		customerIDInt, _ := strconv.ParseInt(r.customerID, 10, 64)

		sgUpdateBody := map[string]interface{}{
			"vttVmId":        instanceIDInt,
			"securityGroups": sgPayload,
			"vpcId":          vpcIDInt,
			"customerId":     customerIDInt,
		}

		if apiResp, diags := callAPI(ctx, r.client, pathSGVmUpdate, sgUpdateBody); diags.HasError() {
			if apiResp != nil && isNotFoundMessage(apiResp.Message) {
				return
			}
			resp.Diagnostics.Append(diags...)
			return
		}

		// Wait for detach to complete (async API)
		deadline := time.Now().Add(2 * time.Minute)
		for {
			sgs, err := r.getCurrentSGs(ctx, instanceID, vpcID)
			if err != nil {
				if err.Error() == "not_found" {
					break
				}
			} else {
				found := false
				for _, sg := range sgs {
					if sg == sgID {
						found = true
						break
					}
				}
				if !found {
					break
				}
			}
			if time.Now().After(deadline) {
				resp.Diagnostics.AddError("Timeout", "Timed out waiting for security group to detach")
				return
			}
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

func (r *SecurityGroupAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *SecurityGroupAttachmentResource) getCurrentSGs(ctx context.Context, vmID, vpcID string) ([]string, error) {
	body := map[string]interface{}{
		"instance_id": vmID,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathVMDetail, body)
	if d.HasError() {
		if apiResp != nil && (isNotFoundMessage(apiResp.Message) || apiResp.Message == "ERROR_VALIDATE_RESOURCE") {
			return nil, fmt.Errorf("not_found")
		}
		return nil, fmt.Errorf("API error: %s", d[0].Detail())
	}

	var data map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}

	var sgIDs []string
	if sgs, ok := data["securityGroups"].([]interface{}); ok {
		for _, sg := range sgs {
			if sgMap, ok := sg.(map[string]interface{}); ok {
				if id := asIDString(sgMap, "vttSecurityGroupId"); id != "" {
					sgIDs = append(sgIDs, id)
				}
			}
		}
	}
	return sgIDs, nil
}
