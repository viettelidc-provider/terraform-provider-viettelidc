// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"fmt"
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
	_ resource.Resource                = &clusterResource{}
	_ resource.ResourceWithConfigure   = &clusterResource{}
	_ resource.ResourceWithImportState = &clusterResource{}
)

func NewClusterResource() resource.Resource {
	return &clusterResource{}
}

type clusterResource struct {
	clientData *providerdata.ProviderData
}

func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_cluster"
}

func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *clusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Provides a VKS Kubernetes Cluster resource.\n\n> **Note:** Creation is explicitly disabled via Terraform because cluster provisioning takes a very long time. Use `terraform import` to manage existing clusters.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the Kubernetes Cluster.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the Kubernetes Cluster.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Status of the Cluster.",
				Computed:    true,
			},
			"version": schema.StringAttribute{
				Description: "Kubernetes version.",
				Optional:    true,
				Computed:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "API endpoint.",
				Computed:    true,
			},
			"vpc_id": schema.StringAttribute{
				Description: "VPC ID of the Cluster.",
				Computed:    true,
			},
		},
	}
}

func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError(
		"Action Not Supported",
		"Creating VKS cluster via Terraform is not supported in this version. Please create the cluster on the Viettel Portal and import it using `terraform import`.",
	)
}

func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"id":          state.ID.ValueString(),
		"customer_id": r.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathClusterDetail, payload)
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
		state.Name = types.StringValue(asString(dataMap, "clusterName"))
		if state.Name.IsNull() || state.Name.ValueString() == "" {
			state.Name = types.StringValue(asString(dataMap, "name"))
		}
		state.Status = types.StringValue(asString(dataMap, "status"))
		state.Version = types.StringValue(asString(dataMap, "version"))
		state.Endpoint = types.StringValue(asString(dataMap, "apiAddress"))
		state.VpcID = types.StringValue(asString(dataMap, "vpcId"))

		if vpcConfig, ok := dataMap["vpcConfig"].(map[string]interface{}); ok {
			state.VpcID = types.StringValue(asString(vpcConfig, "vpcId"))
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddError("Parse Error", "Could not parse cluster detail response data")
	}
}

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Version.IsUnknown() && plan.Version.ValueString() != state.Version.ValueString() {
		upgradePayload := map[string]interface{}{
			"clusterId":   state.ID.ValueString(),
			"version":     plan.Version.ValueString(),
			"customer_id": r.clientData.CustomerID,
		}

		_, diags := callAPI(ctx, r.clientData.Client, pathClusterVersionUpdate, upgradePayload)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Wait up to 30 minutes, checking status every 30 seconds
		for i := 0; i < 60; i++ {
			time.Sleep(30 * time.Second)

			readPayload := map[string]interface{}{
				"id":          state.ID.ValueString(),
				"customer_id": r.clientData.CustomerID,
			}
			readResp, _ := callAPI(ctx, r.clientData.Client, pathClusterDetail, readPayload)
			if readResp != nil && readResp.IsSuccess() {
				var dataMap map[string]interface{}
				if err := json.Unmarshal(readResp.Data, &dataMap); err == nil {
					status := asString(dataMap, "status")
					if status == "ACTIVE" || status == "POWER_ON" {
						plan.Status = types.StringValue(status)
						plan.Version = types.StringValue(asString(dataMap, "version"))
						plan.Endpoint = types.StringValue(asString(dataMap, "apiAddress"))
						plan.VpcID = types.StringValue(asString(dataMap, "vpcId"))
						if vpcConfig, ok := dataMap["vpcConfig"].(map[string]interface{}); ok {
							plan.VpcID = types.StringValue(asString(vpcConfig, "vpcId"))
						}
						break
					}
					if status == "ERROR" || status == "FAILED" {
						resp.Diagnostics.AddError("Upgrade Error", fmt.Sprintf("Cluster upgrade failed: status became %s", status))
						return
					}
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No-op: Cluster deletion is not supported via IaC/Terraform.
	// This cleanly removes the cluster from Terraform state without deleting the physical cluster.
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
