// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	_ resource.Resource                = &addonResource{}
	_ resource.ResourceWithConfigure   = &addonResource{}
	_ resource.ResourceWithImportState = &addonResource{}
)

func NewAddonResource() resource.Resource {
	return &addonResource{}
}

type addonResource struct {
	clientData *providerdata.ProviderData
}

type AddonResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Version   types.String `tfsdk:"version"`
	Status    types.String `tfsdk:"status"`
}

func (r *addonResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_addon"
}

func (r *addonResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *addonResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a VKS Kubernetes Add-on resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the addon resource, formatted as cluster_id/addon_name.",
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
			"name": schema.StringAttribute{
				Description: "Name of the addon (e.g. csi-nfs, prometheus).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				Description: "Version of the addon to install.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Status of the addon.",
				Computed:    true,
			},
		},
	}
}

func (r *addonResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AddonResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query list to find versionId
	listPayload := map[string]interface{}{
		"clusterId":  plan.ClusterID.ValueString(),
		"customerId": r.clientData.CustomerID,
	}
	listResp, diags := callAPI(ctx, r.clientData.Client, pathAddonList, listPayload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var listEnvelope struct {
		Items []map[string]interface{} `json:"items"`
	}
	var versionID int
	versionIDFound := false
	alreadyActive := false
	if err := json.Unmarshal(listResp.Data, &listEnvelope); err == nil {
		for _, item := range listEnvelope.Items {
			addonNameVal := asString(item, "addonName")
			if strings.EqualFold(addonNameVal, plan.Name.ValueString()) {
				if versions, ok := item["versions"].([]interface{}); ok {
					for _, vObj := range versions {
						if vMap, ok := vObj.(map[string]interface{}); ok {
							verName := asString(vMap, "versionName")
							if verName == plan.Version.ValueString() {
								if vidVal, ok := vMap["versionId"]; ok {
									if f, ok := vidVal.(float64); ok {
										versionID = int(f)
										versionIDFound = true
									} else if i, ok := vidVal.(int); ok {
										versionID = i
										versionIDFound = true
									}
								}
								statusVal := asString(vMap, "status")
								if strings.EqualFold(statusVal, "active") {
									alreadyActive = true
								}
								break
							}
						}
					}
				}
				break
			}
		}
	}

	if !versionIDFound {
		resp.Diagnostics.AddError("Addon Version Not Found", fmt.Sprintf("Could not find version %s for addon %s", plan.Version.ValueString(), plan.Name.ValueString()))
		return
	}

	if alreadyActive {
		plan.ID = types.StringValue(fmt.Sprintf("%s/%s", plan.ClusterID.ValueString(), plan.Name.ValueString()))
		plan.Status = types.StringValue("installed")
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	payload := map[string]interface{}{
		"versionId":   versionID,
		"clusterId":   plan.ClusterID.ValueString(),
		"customerId":  r.clientData.CustomerID,
	}

	_, diags = callAPI(ctx, r.clientData.Client, pathAddonInstall, payload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", plan.ClusterID.ValueString(), plan.Name.ValueString()))
	plan.Status = types.StringValue("installed")

	// Poll list API to ensure it's installed
	for i := 0; i < 30; i++ {
		time.Sleep(10 * time.Second)
		listPayload := map[string]interface{}{
			"clusterId":  plan.ClusterID.ValueString(),
			"customerId": r.clientData.CustomerID,
		}
		listResp, _ := callAPI(ctx, r.clientData.Client, pathAddonList, listPayload)
		if listResp != nil && listResp.IsSuccess() {
			var listEnvelope struct {
				Items []map[string]interface{} `json:"items"`
			}
			if err := json.Unmarshal(listResp.Data, &listEnvelope); err == nil {
				found := false
				for _, item := range listEnvelope.Items {
					addonNameVal := asString(item, "addonName")
					if strings.EqualFold(addonNameVal, plan.Name.ValueString()) {
						if versions, ok := item["versions"].([]interface{}); ok {
							for _, vObj := range versions {
								if vMap, ok := vObj.(map[string]interface{}); ok {
									verName := asString(vMap, "versionName")
									statusVal := asString(vMap, "status")
									if verName == plan.Version.ValueString() && statusVal == "active" {
										found = true
										break
									}
								}
							}
						}
						break
					}
				}
				if found {
					break
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *addonResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AddonResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idParts := strings.Split(state.ID.ValueString(), "/")
	if len(idParts) != 2 {
		resp.Diagnostics.AddError("Invalid ID", "Addon ID must be formatted as cluster_id/addon_name")
		return
	}
	clusterID := idParts[0]
	addonName := idParts[1]

	listPayload := map[string]interface{}{
		"clusterId":  clusterID,
		"customerId": r.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathAddonList, listPayload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listEnvelope struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(apiResp.Data, &listEnvelope); err == nil {
		found := false
		for _, item := range listEnvelope.Items {
			addonNameVal := asString(item, "addonName")
			if strings.EqualFold(addonNameVal, addonName) {
				if versions, ok := item["versions"].([]interface{}); ok {
					for _, vObj := range versions {
						if vMap, ok := vObj.(map[string]interface{}); ok {
							statusVal := asString(vMap, "status")
							if statusVal == "active" {
								found = true
								state.Status = types.StringValue("installed")
								state.Version = types.StringValue(asString(vMap, "versionName"))
								break
							}
						}
					}
				}
				break
			}
		}
		if !found {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse addon list response")
	}
}

func (r *addonResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Recreate on change is handled by Schema RequiresReplace
}

func (r *addonResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AddonResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idParts := strings.Split(state.ID.ValueString(), "/")
	if len(idParts) != 2 {
		return
	}
	clusterID := idParts[0]
	addonName := idParts[1]

	// Query addon list to find versionId of the active version
	listPayload := map[string]interface{}{
		"clusterId":  clusterID,
		"customerId": r.clientData.CustomerID,
	}
	listResp, diags := callAPI(ctx, r.clientData.Client, pathAddonList, listPayload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listEnvelope struct {
		Items []map[string]interface{} `json:"items"`
	}
	var versionID int
	versionIDFound := false
	if err := json.Unmarshal(listResp.Data, &listEnvelope); err == nil {
		for _, item := range listEnvelope.Items {
			addonNameVal := asString(item, "addonName")
			if strings.EqualFold(addonNameVal, addonName) {
				if versions, ok := item["versions"].([]interface{}); ok {
					for _, vObj := range versions {
						if vMap, ok := vObj.(map[string]interface{}); ok {
							statusVal := asString(vMap, "status")
							if statusVal == "active" || statusVal == "installing" {
								if vidVal, ok := vMap["versionId"]; ok {
									if f, ok := vidVal.(float64); ok {
										versionID = int(f)
										versionIDFound = true
									} else if i, ok := vidVal.(int); ok {
										versionID = i
										versionIDFound = true
									} else if s, ok := vidVal.(string); ok {
										if parsed, err := strconv.Atoi(s); err == nil {
											versionID = parsed
											versionIDFound = true
										}
									}
								}
								break
							}
						}
					}
				}
				break
			}
		}
	}

	if !versionIDFound {
		return
	}

	payload := map[string]interface{}{
		"versionId":   versionID,
		"clusterId":   clusterID,
		"customerId":  r.clientData.CustomerID,
		"addonName":   addonName,
	}
	_, deleteDiags := callAPI(ctx, r.clientData.Client, pathAddonUninstall, payload)
	if deleteDiags.HasError() {
		resp.Diagnostics.AddWarning(
			"Uninstall Addon Warning",
			fmt.Sprintf("Failed to call addon uninstall (might be already uninstalled): %v", deleteDiags),
		)
		// If it failed to initiate, we shouldn't poll because it might not be doing anything.
		return
	}

	// Poll list API to ensure it's uninstalled
	for i := 0; i < 30; i++ {
		time.Sleep(10 * time.Second)
		pollResp, _ := callAPI(ctx, r.clientData.Client, pathAddonList, listPayload)
		if pollResp != nil && pollResp.IsSuccess() {
			var env struct {
				Items []map[string]interface{} `json:"items"`
			}
			if err := json.Unmarshal(pollResp.Data, &env); err == nil {
				foundActive := false
				for _, item := range env.Items {
					an := asString(item, "addonName")
					if strings.EqualFold(an, addonName) {
						if versions, ok := item["versions"].([]interface{}); ok {
							for _, vObj := range versions {
								if vMap, ok := vObj.(map[string]interface{}); ok {
									st := asString(vMap, "status")
									// active or uninstalling means it's still there
									if st == "active" || st == "uninstalling" {
										foundActive = true
									}
								}
							}
						}
					}
				}
				if !foundActive {
					return
				}
			}
		}
	}
	resp.Diagnostics.AddWarning("Addon Uninstall Timeout", "Addon did not disappear after 5 minutes")
}

func (r *addonResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
