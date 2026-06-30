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
	_ resource.Resource                = &nfsStorageResource{}
	_ resource.ResourceWithConfigure   = &nfsStorageResource{}
	_ resource.ResourceWithImportState = &nfsStorageResource{}
)

func NewNfsStorageResource() resource.Resource {
	return &nfsStorageResource{}
}

type nfsStorageResource struct {
	clientData *providerdata.ProviderData
}

type NfsStorageResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	StorageSize types.Int64  `tfsdk:"storage_size"`
	Status      types.String `tfsdk:"status"`
}

func (r *nfsStorageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_nfs_storage"
}

func (r *nfsStorageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *nfsStorageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a VKS NFS Storage resource. Note: Creation is explicitly disabled via Terraform. Use `terraform import` to manage existing NFS Storage.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the NFS storage resource, formatted as cluster_id/nfs.",
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
			"storage_size": schema.Int64Attribute{
				Description: "Size of the NFS storage in GB.",
				Required:    true,
			},
			"status": schema.StringAttribute{
				Description: "Status of the NFS storage.",
				Computed:    true,
			},
		},
	}
}

func (r *nfsStorageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError(
		"Action Not Supported",
		"Creating VKS NFS Storage via Terraform is not supported in this version. Please enable NFS Storage on the Viettel Portal and import it using `terraform import`.",
	)
}

func (r *nfsStorageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NfsStorageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idParts := strings.Split(state.ID.ValueString(), "/")
	clusterID := idParts[0]

	payload := map[string]interface{}{
		"clusterId":  clusterID,
		"customerId": r.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathClusterNFSDetail, payload)
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
		state.ClusterID = types.StringValue(clusterID)
		state.Status = types.StringValue(asString(dataMap, "status"))
		state.StorageSize = types.Int64Value(asInt64(dataMap, "addOnsStorage"))
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse NFS detail response")
	}
}

func (r *nfsStorageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state NfsStorageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":   state.ClusterID.ValueString(),
		"customer_id":  r.clientData.CustomerID,
		"storage_size": plan.StorageSize.ValueInt64(),
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathClusterNFSAddons, payload)
	if diags.HasError() {
		if apiResp != nil && strings.Contains(apiResp.Message, "ERROR_NODE_CAN_NOT_EXECUTE") {
			resp.Diagnostics.AddWarning(
				"NFS Storage Resizing Warning",
				fmt.Sprintf("NFS Node is in ERROR state on portal; ignoring resize API failure to allow Terraform state update: %s", apiResp.Message),
			)
		} else {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	plan.Status = types.StringValue("AVAILABLE")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nfsStorageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NfsStorageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idParts := strings.Split(state.ID.ValueString(), "/")
	clusterID := idParts[0]

	payload := map[string]interface{}{
		"name":       "csi-nfs",
		"clusterId":  clusterID,
		"customerId": r.clientData.CustomerID,
	}

	_, diags := callAPI(ctx, r.clientData.Client, pathAddonUninstall, payload)
	if diags.HasError() {
		resp.Diagnostics.AddWarning(
			"Uninstall NFS Storage Warning",
			fmt.Sprintf("Failed to call NFS storage uninstall (might be already uninstalled): %v", diags),
		)
		return
	}

	// Poll list API to ensure it's uninstalled
	for i := 0; i < 30; i++ {
		time.Sleep(10 * time.Second)
		listPayload := map[string]interface{}{
			"clusterId":  clusterID,
			"customerId": r.clientData.CustomerID,
		}
		pollResp, _ := callAPI(ctx, r.clientData.Client, pathAddonList, listPayload)
		if pollResp != nil && pollResp.IsSuccess() {
			var env struct {
				Items []map[string]interface{} `json:"items"`
			}
			if err := json.Unmarshal(pollResp.Data, &env); err == nil {
				foundActive := false
				for _, item := range env.Items {
					an := asString(item, "addonName")
					if strings.EqualFold(an, "csi-nfs") {
						if versions, ok := item["versions"].([]interface{}); ok {
							for _, vObj := range versions {
								if vMap, ok := vObj.(map[string]interface{}); ok {
									st := asString(vMap, "status")
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
	resp.Diagnostics.AddWarning("NFS Storage Uninstall Timeout", "NFS Storage did not disappear after 5 minutes")
}

func (r *nfsStorageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	} else {
		resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	}
}
