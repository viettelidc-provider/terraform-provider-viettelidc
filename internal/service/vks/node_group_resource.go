// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

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
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

var (
	_ resource.Resource                = &nodeGroupResource{}
	_ resource.ResourceWithConfigure   = &nodeGroupResource{}
	_ resource.ResourceWithImportState = &nodeGroupResource{}
)

func NewNodeGroupResource() resource.Resource {
	return &nodeGroupResource{}
}

type nodeGroupResource struct {
	clientData *providerdata.ProviderData
}

func (r *nodeGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_node_group"
}

func (r *nodeGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *nodeGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Provides a VKS Node Group resource.\n\n> **Note:** Creation is explicitly disabled via Terraform. Use `terraform import` to manage existing Node Groups.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the Node Group.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Description: "ID of the K8s Cluster.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the Node Group.",
				Required:    true,
			},
			"instance_type": schema.StringAttribute{
				Description: "Instance type of the workers.",
				Required:    true,
			},
			"min_size": schema.Int64Attribute{
				MarkdownDescription: "Minimum number of nodes in the Node Group. Cannot be less than 1.",
				Required:    true,
			},
			"max_size": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of nodes in the Node Group.",
				Required:    true,
			},
			"desired_size": schema.Int64Attribute{
				MarkdownDescription: "Desired number of nodes in the Node Group. Must be between minimum and maximum size.",
				Required:    true,
			},
			"status": schema.StringAttribute{
				Description: "Status of the Node Group.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *nodeGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError(
		"Action Not Supported",
		"Creating VKS Node Group via Terraform is not supported in this version. Please create the Node Group on the Viettel Portal and import it using `terraform import`.",
	)
}

func (r *nodeGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NodeGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	nodeGroupID := state.ID.ValueString()
	if strings.Contains(nodeGroupID, "/") {
		parts := strings.Split(nodeGroupID, "/")
		clusterID = parts[0]
		nodeGroupID = parts[1]
	}

	payload := map[string]interface{}{
		"id":          nodeGroupID,
		"cluster_id":  clusterID,
		"customer_id": r.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathNodeGroupDetail, payload)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		if dataMap == nil {
			resp.State.RemoveResource(ctx)
			return
		}
		state.Name = types.StringValue(asString(dataMap, "name"))
		state.Status = types.StringValue(asString(dataMap, "status"))
		if cId := asString(dataMap, "clusterId"); cId != "" {
			state.ClusterID = types.StringValue(cId)
		} else if cId := asString(dataMap, "vttClusterId"); cId != "" {
			state.ClusterID = types.StringValue(cId)
		}
		instType := asString(dataMap, "instanceType")
		if instType == "" {
			instType = asString(dataMap, "code")
		}
		if instType != "" {
			state.InstanceType = types.StringValue(instType)
		}

		minSize := asInt64(dataMap, "minSize")
		if minSize == 0 {
			minSize = asInt64(dataMap, "minNode")
		}
		state.MinSize = types.Int64Value(minSize)

		maxSize := asInt64(dataMap, "maxSize")
		if maxSize == 0 {
			maxSize = asInt64(dataMap, "maxNode")
		}
		state.MaxSize = types.Int64Value(maxSize)

		desiredSize := asInt64(dataMap, "desiredSize")
		if desiredSize == 0 {
			desiredSize = asInt64(dataMap, "replicas")
		}
		state.DesiredSize = types.Int64Value(desiredSize)

		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (r *nodeGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state NodeGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"customer_id":   r.clientData.CustomerID,
		"cluster_id":    state.ClusterID.ValueString(),
		"id":            state.ID.ValueString(),
		"min_size":      plan.MinSize.ValueInt64(),
		"max_size":      plan.MaxSize.ValueInt64(),
		"desired_size":  plan.DesiredSize.ValueInt64(),
		"is_auto_scale": plan.MinSize.ValueInt64() != plan.MaxSize.ValueInt64(),
		"name":          state.Name.ValueString(),
		"isAutoRepair":  false,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathNodeGroupEdit, payload)
	if diags.HasError() {
		if apiResp != nil && (strings.Contains(apiResp.Message, "ERROR_COMMON_0001") || strings.Contains(apiResp.Message, "ERROR_CLUSTER_UPDATING")) {
			resp.Diagnostics.AddWarning(
				"Node Group Scaling Warning",
				fmt.Sprintf("Cluster node group is degraded, busy or updating. Ignoring API failure to allow Terraform state update: %s", apiResp.Message),
			)
		} else {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	// Async polling loop
	for i := 0; i < 90; i++ {
		time.Sleep(10 * time.Second)
		readPayload := map[string]interface{}{
			"id":          state.ID.ValueString(),
			"cluster_id":  state.ClusterID.ValueString(),
			"customer_id": r.clientData.CustomerID,
		}
		readResp, _ := callAPI(ctx, r.clientData.Client, pathNodeGroupDetail, readPayload)
		if readResp != nil && readResp.IsSuccess() {
			var dMap map[string]interface{}
			if err := json.Unmarshal(readResp.Data, &dMap); err == nil {
				status := asString(dMap, "status")
				if status == "" || status == "ACTIVE" || status == "POWER_ON" {
					plan.Status = types.StringValue(status)
					break
				}
				if status == "ERROR" {
					resp.Diagnostics.AddError("Update Error", "Node Group reached ERROR state")
					return
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nodeGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NodeGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"nodeGroupId": state.ID.ValueString(),
		"cluster_id":  state.ClusterID.ValueString(),
		"customer_id": r.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathNodeGroupDelete, payload)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until deleted
	for i := 0; i < 90; i++ { // 15 mins timeout
		time.Sleep(10 * time.Second)
		readPayload := map[string]interface{}{
			"id":          state.ID.ValueString(),
			"cluster_id":  state.ClusterID.ValueString(),
			"customer_id": r.clientData.CustomerID,
		}
		readResp, _ := callAPI(ctx, r.clientData.Client, pathNodeGroupDetail, readPayload)
		if readResp != nil && isNotFoundMessage(readResp.Message) {
			return
		}
	}
	resp.Diagnostics.AddError("Delete Timeout", "Node Group was not deleted within the expected time.")
}

func (r *nodeGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
	} else {
		resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	}
}
