// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

type nfsServerDataSource struct {
	clientData *providerdata.ProviderData
}

type NfsServerDSModel struct {
	ID         types.String `tfsdk:"id"`
	ClusterID  types.String `tfsdk:"cluster_id"`
	ServerIP   types.String `tfsdk:"server_ip"`
	ExportPath types.String `tfsdk:"export_path"`
	Status     types.String `tfsdk:"status"`
}

func NewNfsServerDataSource() datasource.DataSource {
	return &nfsServerDataSource{}
}

func (d *nfsServerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_nfs_server"
}

func (d *nfsServerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *nfsServerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS NFS Server details.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"server_ip": schema.StringAttribute{
				Computed: true,
			},
			"export_path": schema.StringAttribute{
				Computed: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *nfsServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NfsServerDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"clusterId":  config.ClusterID.ValueString(),
		"customerId": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathNFSServerDetail, payload)
	if diags.HasError() {
		resp.Diagnostics.AddWarning(
			"NFS Server Details Query Failed",
			fmt.Sprintf("Failed to query NFS server details (path=%s): %v. Using default/empty values.", pathNFSServerDetail, diags),
		)
		config.ID = config.ClusterID
		config.ServerIP = types.StringValue("")
		config.ExportPath = types.StringValue("")
		config.Status = types.StringValue("UNKNOWN")
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = config.ClusterID
		config.ServerIP = types.StringValue(asString(dataMap, "nfsServerIp"))
		if config.ServerIP.IsNull() || config.ServerIP.ValueString() == "" {
			config.ServerIP = types.StringValue(asString(dataMap, "serverIp"))
		}
		config.ExportPath = types.StringValue(asString(dataMap, "exportPath"))
		config.Status = types.StringValue(asString(dataMap, "status"))
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	} else {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse NFS server details response")
	}
}
