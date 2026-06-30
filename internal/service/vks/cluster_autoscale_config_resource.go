// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

var (
	_ resource.Resource                = &clusterAutoscaleConfigResource{}
	_ resource.ResourceWithConfigure   = &clusterAutoscaleConfigResource{}
	_ resource.ResourceWithImportState = &clusterAutoscaleConfigResource{}
)

func NewClusterAutoscaleConfigResource() resource.Resource {
	return &clusterAutoscaleConfigResource{}
}

type clusterAutoscaleConfigResource struct {
	clientData *providerdata.ProviderData
}

type ClusterAutoscaleConfigResourceModel struct {
	ID                            types.String `tfsdk:"id"`
	ClusterID                     types.String `tfsdk:"cluster_id"`
	ScaleDownDelayAfterAdd        types.String `tfsdk:"scale_down_delay_after_add"`
	ScaleDownDelayAfterDelete     types.String `tfsdk:"scale_down_delay_after_delete"`
	ScaleDownDelayAfterFailure    types.String `tfsdk:"scale_down_delay_after_failure"`
	ScaleDownUnneededTime         types.String `tfsdk:"scale_down_unneeded_time"`
	ScaleDownUtilizationThreshold types.String `tfsdk:"scale_down_utilization_threshold"`
	ScanInterval                  types.String `tfsdk:"scan_interval"`
}

func (r *clusterAutoscaleConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_cluster_autoscale_config"
}

func (r *clusterAutoscaleConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *clusterAutoscaleConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a VKS Cluster Autoscale Config resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the config, same as cluster_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Description: "ID of the Kubernetes Cluster.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scale_down_delay_after_add": schema.StringAttribute{
				MarkdownDescription: "How long after scale up that scale down evaluation resumes. Unit: seconds. Default is 300.",
				Optional:            true,
				Computed:            true,
			},
			"scale_down_delay_after_delete": schema.StringAttribute{
				MarkdownDescription: "How long after node deletion that scale down evaluation resumes. Unit: seconds. Default is 2.",
				Optional:            true,
				Computed:            true,
			},
			"scale_down_delay_after_failure": schema.StringAttribute{
				MarkdownDescription: "How long after scale down failure that scale down evaluation resumes. Unit: seconds. Default is 120.",
				Optional:            true,
				Computed:            true,
			},
			"scale_down_unneeded_time": schema.StringAttribute{
				MarkdownDescription: "How long a node should be unneeded before it is eligible for scale down. Unit: seconds. Default is 120.",
				Optional:            true,
				Computed:            true,
			},
			"scale_down_utilization_threshold": schema.StringAttribute{
				MarkdownDescription: "Node utilization level, defined as sum of requested resources divided by capacity, below which a node can be considered for scale down. Range: 0.0 to 1.0. Default is 0.1.",
				Optional:            true,
				Computed:            true,
			},
			"scan_interval": schema.StringAttribute{
				MarkdownDescription: "How often cluster is reevaluated for scale up or down. Unit: seconds. Default is 10.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *clusterAutoscaleConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ClusterAutoscaleConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags := r.updateAutoscaleConfig(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = plan.ClusterID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterAutoscaleConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ClusterAutoscaleConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterIDNum, err := strconv.ParseInt(state.ClusterID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Cluster ID", fmt.Sprintf("Failed to parse cluster_id: %s", err))
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  clusterIDNum,
		"customer_id": r.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathClusterAutoscaleConfigDetail, payload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var values []map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &values); err != nil {
		resp.Diagnostics.AddError("Decode Detail Error", err.Error())
		return
	}

	for _, v := range values {
		key := asString(v, "key")
		val := asString(v, "value")

		tv := types.StringValue(val)
		switch key {
		case "scale-down-delay-after-add":
			state.ScaleDownDelayAfterAdd = tv
		case "scale-down-delay-after-delete":
			state.ScaleDownDelayAfterDelete = tv
		case "scale-down-delay-after-failure":
			state.ScaleDownDelayAfterFailure = tv
		case "scale-down-unneeded-time":
			state.ScaleDownUnneededTime = tv
		case "scale-down-utilization-threshold":
			state.ScaleDownUtilizationThreshold = tv
		case "scan-interval":
			state.ScanInterval = tv
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterAutoscaleConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ClusterAutoscaleConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags := r.updateAutoscaleConfig(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterAutoscaleConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// The user requested that DELETE should not call any API,
	// and should only remove the resource from Terraform state.
	// The Terraform Plugin Framework automatically removes the resource from state
	// as long as no errors are returned here.
}

func (r *clusterAutoscaleConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_id"), req, resp)
}

func (r *clusterAutoscaleConfigResource) updateAutoscaleConfig(ctx context.Context, plan ClusterAutoscaleConfigResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	clusterIDNum, err := strconv.ParseInt(plan.ClusterID.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("Invalid Cluster ID", fmt.Sprintf("Failed to parse cluster_id: %s", err))
		return diags
	}

	values := []map[string]interface{}{}
	addValue := func(key, val, unit, def string) {
		v := val
		if v == "" {
			v = def
		}
		values = append(values, map[string]interface{}{
			"key":   key,
			"value": v,
			"unit":  unit,
		})
	}

	addValue("scale-down-delay-after-add", plan.ScaleDownDelayAfterAdd.ValueString(), "s", "300")
	addValue("scale-down-delay-after-delete", plan.ScaleDownDelayAfterDelete.ValueString(), "s", "2")
	addValue("scale-down-delay-after-failure", plan.ScaleDownDelayAfterFailure.ValueString(), "s", "120")
	addValue("scale-down-unneeded-time", plan.ScaleDownUnneededTime.ValueString(), "s", "120")
	addValue("scale-down-utilization-threshold", plan.ScaleDownUtilizationThreshold.ValueString(), "", "0.1")
	addValue("scan-interval", plan.ScanInterval.ValueString(), "s", "10")

	payload := map[string]interface{}{
		"cluster_id":  clusterIDNum,
		"customer_id": r.clientData.CustomerID,
		"values":      values,
	}

	_, apiDiags := callAPI(ctx, r.clientData.Client, pathClusterAutoscaleConfigUpdate, payload)
	diags.Append(apiDiags...)

	return diags
}
