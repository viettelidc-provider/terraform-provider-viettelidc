// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/viettelidc-provider/viettelidc-api-client-go/service/voks"
	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"
	"strconv"
	"strings"
	"time"
)

var (
	_ resource.Resource                = &addonResource{}
	_ resource.ResourceWithConfigure   = &addonResource{}
	_ resource.ResourceWithImportState = &addonResource{}
)

type addonResource struct {
	client *voks.APIClient
}

type AddonResourceModel struct {
	ClusterId types.Int32  `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Version   types.String `tfsdk:"version"`
	Status    types.String `tfsdk:"status"`
}

func NewAddonResource() resource.Resource {
	return &addonResource{}
}

func (a *addonResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	// Add a nil check when handling ProviderData because Terraform
	// sets that data after it calls the ConfigureProvider RPC.
	if request.ProviderData == nil {
		return
	}

	cfg, ok := request.ProviderData.(*viettelidc.Configuration)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *apiclient.Client, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)

		return
	}

	a.client = voks.NewAPIClient(*cfg)
}

func (a *addonResource) Metadata(ctx context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_addon"
}

func (a *addonResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Manage a Kubernetes Add-on resource within ViettelIdc.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.Int32Attribute{
				Description: "Id of the Cluster.",
				Required: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the Add-on.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				Description: "Version of Add-on.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "The current status of Add-on. Valid values: `ACTIVE`, `INACTIVE`, `INSTALLING`, `UNINSTALLING`.",
				Computed: true,
			},
		},
	}
}

func (a *addonResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {

	var plan AddonResourceModel
	diags := request.Plan.Get(ctx, &plan)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	exitingAddon, _, err := a.client.AddOnApi.GetDetailAddon(ctx, voks.GetDetailAddonRequest{
		ClusterId: plan.ClusterId.ValueInt32(),
		Name:      plan.Name.ValueString(),
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Error validating Cluster Addon status",
			"Could not validate Cluster Addon status, unexpected error: "+err.Error())
		return
	}

	if exitingAddon.Status != "inactive" {
		response.Diagnostics.AddError(
			"Error Cluster Addon already installed",
			"This Cluster Addon already installed: "+exitingAddon.Name+" is in "+exitingAddon.Status+" status")
		return
	}

	_, err = a.client.AddOnApi.InstallAddOn(ctx, voks.AddonInstallRequest{
		ClusterId: plan.ClusterId.ValueInt32(),
		Name:      plan.Name.ValueString(),
		Version:   plan.Version.ValueString(),
	})

	if err != nil {
		response.Diagnostics.AddError(
			"Error installing Cluster Addon",
			"Could not create Cluster Addon, unexpected error: "+err.Error())
		return
	}

	for {
		detailRes, _, err := a.client.AddOnApi.GetDetailAddon(ctx, voks.GetDetailAddonRequest{
			ClusterId: plan.ClusterId.ValueInt32(),
			Name:      plan.Name.ValueString(),
		})
		if err != nil {
			response.Diagnostics.AddError(
				"Error updating Cluster Addon status",
				"Could not update Cluster Addon status, unexpected error: "+err.Error())
			return
		}
		if detailRes.Status == "active" {
			plan.Status = types.StringValue(detailRes.Status)
			break
		}
		time.Sleep(10 * time.Second)
	}

	// Set state to fully populated data
	diags = response.State.Set(ctx, &plan)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
}

func (a *addonResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {

	var state AddonResourceModel
	diags := request.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	addonRes, _, err := a.client.AddOnApi.GetDetailAddon(ctx, voks.GetDetailAddonRequest{
		ClusterId: state.ClusterId.ValueInt32(),
		Name:      state.Name.ValueString(),
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster Addon detail",
			"Could not read Cluster Addon detail, unexpected error: "+err.Error())
		return
	}

	// Overwrite Addon with refresh state
	state.Version = types.StringValue(addonRes.Version)
	state.Status = types.StringValue(addonRes.Status)

	diags = response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}

func (a *addonResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	idParts := strings.Split(request.ID, ",")

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		response.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: cluster_id,name. Got: %q", request.ID),
		)
		return
	}

	clusterId, err := strconv.ParseInt(idParts[0], 10, 32)
	if err != nil {
		response.Diagnostics.AddError(
			"Error parsing Cluster ID",
			"Could not parse Cluster ID, unexpected error: "+err.Error())
		return
	}

	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("cluster_id"), clusterId)...)
	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("name"), idParts[1])...)
}

func (a *addonResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	response.Diagnostics.AddError(
		"Error updating Cluster Addon",
		"Could not update Cluster Addon, this action is not supported.")
}

func (a *addonResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {

	var state AddonResourceModel
	diags := request.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	_, err := a.client.AddOnApi.UninstallAddOn(ctx, voks.AddonUninstallRequest{
		ClusterId: state.ClusterId.ValueInt32(),
		Name:      state.Name.ValueString(),
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Error uninstall Cluster Addon",
			"Could not uninstall Cluster Addon, unexpected error: "+err.Error())
		return
	}

	for {
		detailRes, _, err := a.client.AddOnApi.GetDetailAddon(ctx, voks.GetDetailAddonRequest{
			ClusterId: state.ClusterId.ValueInt32(),
			Name:      state.Name.ValueString(),
		})
		if err != nil {
			response.Diagnostics.AddError(
				"Error updating Cluster Addon status",
				"Could not update Cluster Addon status, unexpected error: "+err.Error())
			return
		}
		if detailRes.Status == "inactive" {
			state.Status = types.StringValue(detailRes.Status)
			break
		}
		time.Sleep(10 * time.Second)
	}
}
