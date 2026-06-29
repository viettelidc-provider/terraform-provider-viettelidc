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

// --- SUBMENTS DATA SOURCE ---
type subnetsDataSource struct {
	clientData *providerdata.ProviderData
}

type SubnetModel struct {
	ID           types.String `json:"id" tfsdk:"id"`
	Name         types.String `json:"name" tfsdk:"name"`
	CidrBlock    types.String `json:"cidr_block" tfsdk:"cidr_block"`
	IsPublicZone types.Bool   `json:"is_public_zone" tfsdk:"is_public_zone"`
}

type SubnetsDSModel struct {
	ID        types.String  `tfsdk:"id"`
	ClusterID types.String  `tfsdk:"cluster_id"`
	Subnets   []SubnetModel `tfsdk:"subnets"`
}

func NewSubnetsDataSource() datasource.DataSource {
	return &subnetsDataSource{}
}

func (d *subnetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_subnets"
}

func (d *subnetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *subnetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing VKS Cluster Subnets.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"subnets": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"cidr_block": schema.StringAttribute{
							Computed: true,
						},
						"is_public_zone": schema.BoolAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *subnetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SubnetsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterSubnetsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		config.ID = config.ClusterID
		config.Subnets = make([]SubnetModel, 0)
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					config.Subnets = append(config.Subnets, SubnetModel{
						ID:           types.StringValue(asString(m, "subnetId")),
						Name:         types.StringValue(asString(m, "subnetName")),
						CidrBlock:    types.StringValue(asString(m, "Ipv4CIDR")),
						IsPublicZone: types.BoolValue(m["isPublic"] == true),
					})
				}
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- SUBNET DATA SOURCE ---
type subnetDataSource struct {
	clientData *providerdata.ProviderData
}

type SubnetDSModel struct {
	ID           types.String `tfsdk:"id"`
	ClusterID    types.String `tfsdk:"cluster_id"`
	SubnetID     types.String `tfsdk:"subnet_id"`
	Name         types.String `tfsdk:"name"`
	CidrBlock    types.String `tfsdk:"cidr_block"`
	IsPublicZone types.Bool   `tfsdk:"is_public_zone"`
}

func NewSubnetDataSource() datasource.DataSource {
	return &subnetDataSource{}
}

func (d *subnetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_subnet"
}

func (d *subnetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *subnetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for retrieving details of a single VKS Cluster Subnet.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"subnet_id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"cidr_block": schema.StringAttribute{
				Computed: true,
			},
			"is_public_zone": schema.BoolAttribute{
				Computed: true,
			},
		},
	}
}

func (d *subnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SubnetDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	// Read from list endpoint since detail returns empty on prod
	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterSubnetsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					sid := asString(m, "subnetId")
					if sid == config.SubnetID.ValueString() {
						config.ID = types.StringValue(config.ClusterID.ValueString() + "/" + config.SubnetID.ValueString())
						config.Name = types.StringValue(asString(m, "subnetName"))
						config.CidrBlock = types.StringValue(asString(m, "Ipv4CIDR"))
						config.IsPublicZone = types.BoolValue(m["isPublic"] == true)
						resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
						return
					}
				}
			}
		}
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Subnet %s not found in cluster", config.SubnetID.ValueString()))
	}
}

// --- NETWORK INTERFACES DATA SOURCE ---
type networkInterfacesDataSource struct {
	clientData *providerdata.ProviderData
}

type NicModel struct {
	ID        types.String `json:"id" tfsdk:"id"`
	Name      types.String `json:"name" tfsdk:"name"`
	IPAddress types.String `json:"ip_address" tfsdk:"ip_address"`
	Status    types.String `json:"status" tfsdk:"status"`
}

type NetworkInterfacesDSModel struct {
	ID                types.String `tfsdk:"id"`
	ClusterID         types.String `tfsdk:"cluster_id"`
	NetworkInterfaces []NicModel   `tfsdk:"network_interfaces"`
}

func NewNetworkInterfacesDataSource() datasource.DataSource {
	return &networkInterfacesDataSource{}
}

func (d *networkInterfacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_network_interfaces"
}

func (d *networkInterfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *networkInterfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing VKS Cluster Network Interfaces.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"network_interfaces": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"ip_address": schema.StringAttribute{
							Computed: true,
						},
						"status": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *networkInterfacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NetworkInterfacesDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterNICsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		config.ID = config.ClusterID
		config.NetworkInterfaces = make([]NicModel, 0)
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					ip := asString(m, "ipAddress")
					if ip == "" {
						ip = asString(m, "ip")
					}
					config.NetworkInterfaces = append(config.NetworkInterfaces, NicModel{
						ID:        types.StringValue(asString(m, "networkInterfaceId")),
						Name:      types.StringValue(asString(m, "networkInterfaceName")),
						IPAddress: types.StringValue(ip),
						Status:    types.StringValue(asString(m, "status")),
					})
				}
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- NETWORK INTERFACE DATA SOURCE ---
type networkInterfaceDataSource struct {
	clientData *providerdata.ProviderData
}

type NetworkInterfaceDSModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	NicID     types.String `tfsdk:"nic_id"`
	Name      types.String `tfsdk:"name"`
	IPAddress types.String `tfsdk:"ip_address"`
	Status    types.String `tfsdk:"status"`
}

func NewNetworkInterfaceDataSource() datasource.DataSource {
	return &networkInterfaceDataSource{}
}

func (d *networkInterfaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_network_interface"
}

func (d *networkInterfaceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *networkInterfaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for retrieving details of a single VKS Cluster Network Interface.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"nic_id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"ip_address": schema.StringAttribute{
				Computed: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *networkInterfaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NetworkInterfaceDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	// Read from list endpoint since detail returns empty on prod
	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterNICsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					nid := asString(m, "networkInterfaceId")
					if nid == config.NicID.ValueString() {
						ip := asString(m, "ipAddress")
						if ip == "" {
							ip = asString(m, "ip")
						}
						config.ID = types.StringValue(config.ClusterID.ValueString() + "/" + config.NicID.ValueString())
						config.Name = types.StringValue(asString(m, "networkInterfaceName"))
						config.IPAddress = types.StringValue(ip)
						config.Status = types.StringValue(asString(m, "status"))
						resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
						return
					}
				}
			}
		}
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("NIC %s not found in cluster", config.NicID.ValueString()))
	}
}

// --- SECURITY GROUPS DATA SOURCE ---
type securityGroupsDataSource struct {
	clientData *providerdata.ProviderData
}

type SgModel struct {
	ID          types.String `json:"id" tfsdk:"id"`
	Name        types.String `json:"name" tfsdk:"name"`
	Description types.String `json:"description" tfsdk:"description"`
}

type SecurityGroupsDSModel struct {
	ID             types.String `tfsdk:"id"`
	ClusterID      types.String `tfsdk:"cluster_id"`
	SecurityGroups []SgModel    `tfsdk:"security_groups"`
}

func NewSecurityGroupsDataSource() datasource.DataSource {
	return &securityGroupsDataSource{}
}

func (d *securityGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_security_groups"
}

func (d *securityGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *securityGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing VKS Cluster Security Groups.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"security_groups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"description": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *securityGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SecurityGroupsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterSGsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		config.ID = config.ClusterID
		config.SecurityGroups = make([]SgModel, 0)
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					config.SecurityGroups = append(config.SecurityGroups, SgModel{
						ID:          types.StringValue(asString(m, "vttSecurityGroupId")),
						Name:        types.StringValue(asString(m, "name")),
						Description: types.StringValue(asString(m, "description")),
					})
				}
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- SECURITY GROUP DATA SOURCE ---
type securityGroupDataSource struct {
	clientData *providerdata.ProviderData
}

type SecurityGroupDSModel struct {
	ID          types.String `tfsdk:"id"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	SgID        types.String `tfsdk:"sg_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func NewSecurityGroupDataSource() datasource.DataSource {
	return &securityGroupDataSource{}
}

func (d *securityGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_security_group"
}

func (d *securityGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *securityGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for retrieving details of a single VKS Cluster Security Group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"sg_id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"description": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *securityGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SecurityGroupDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"clusterId":  config.ClusterID.ValueString(),
		"sgId":       config.SgID.ValueString(),
		"customerId": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterSGsDetail, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = types.StringValue(config.ClusterID.ValueString() + "/" + config.SgID.ValueString())
		config.Name = types.StringValue(asString(dataMap, "name"))
		config.Description = types.StringValue(asString(dataMap, "description"))
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}
