// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package datasource

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/viettelidc-provider/viettelidc-api-client-go/service/voks"
	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"
)

type clusterDatasource struct {
	client *voks.APIClient
}

type ClusterDataSourceModel struct {
	ID        types.Int32     `tfsdk:"id"`
	Name      types.String    `tfsdk:"name"`
	Status    types.String    `tfsdk:"status"`
	Version   types.String    `tfsdk:"version"`
	Endpoint  types.String    `tfsdk:"endpoint"`
	Nfs       *NfsBlock       `tfsdk:"nfs"`
	VpcConfig *VpcConfigBlock `tfsdk:"vpc_config"`
}

type VpcConfigBlock struct {
	VpcId            types.Int32 `tfsdk:"vpc_id"`
	SecurityGroupIds types.List  `tfsdk:"security_group_ids"`
	SubnetIds        types.List  `tfsdk:"subnet_ids"`
}

type NfsBlock struct {
	Cpu              types.Float64 `tfsdk:"cpu"`
	Memory           types.Float64 `tfsdk:"memory"`
	TotalStorageSize types.Float64 `tfsdk:"total_storage_size"`
	Status           types.String  `tfsdk:"status"`
	IpAddress        types.String  `tfsdk:"ip_address"`
}

var (
	_ datasource.DataSource              = &clusterDatasource{}
	_ datasource.DataSourceWithConfigure = &clusterDatasource{}
)

func NewClusterDataSource() datasource.DataSource {
	return &clusterDatasource{}
}

func (c *clusterDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

	c.client = voks.NewAPIClient(*cfg)
}

func (c *clusterDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_cluster"
}

func (c *clusterDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Retrieve information about a vOKS Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int32Attribute{
				Description: "Id of the Cluster.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the Cluster.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of Cluster. Valid values: `POWER_ON`, `POWER_OFF`, `ERROR`.",
				Computed:    true,
			},
			"version": schema.StringAttribute{
				Description: "Version of Cluster.",
				Computed:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "Endpoint is IP address and port number that define the backend pod associated with a vOKS service.",
				Computed:    true,
			},
			"nfs": schema.SingleNestedAttribute{
				Description: "NFS storage enables multiple nodes in the cluster to access the same file system over a network.",
				Optional:    true,
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"cpu": schema.Float64Attribute{
						Description: "The CPU size of NFS server.",
						Computed:    true,
					},
					"memory": schema.Float64Attribute{
						Description: "The memory size of NFS server.",
						Computed:    true,
					},
					"total_storage_size": schema.Float64Attribute{
						Description: "The size allocated for NFS volumes.",
						Computed:    true,
					},
					"status": schema.StringAttribute{
						Description: "Status of Cluster NFS Storage. When the NFS Storage is present in Terraform, its status will always be `POWER_ON`. Valid values: `POWER_ON`, `UPDATING`, `ERROR`.",
						Computed:    true,
					},
					"ip_address": schema.StringAttribute{
						Description: "Internal IP of NFS server that can be accessed by internal network of your Cluster.",
						Computed:    true,
					},
				},
			},
		},
		Blocks: map[string]schema.Block{
			"vpc_config": schema.SingleNestedBlock{
				Description: "The vpc_config is a configuration that helps define the networking setup for the ViettelIdc Kubernetes Cluster.",
				Attributes: map[string]schema.Attribute{
					"vpc_id": schema.Int32Attribute{
						Description: "ID of the VPC associated with your cluster.",
						Computed:    true,
					},
					"security_group_ids": schema.ListAttribute{
						Description: "The IDs of the security group to be associated with the VPC endpoint.",
						Computed:    true,
						ElementType: types.Int32Type,
					},
					"subnet_ids": schema.ListAttribute{
						Description: "The IDs of the subnets to be associated with the VPC endpoint.",
						Computed:    true,
						ElementType: types.Int32Type,
					},
				},
			},
		},
	}
}

func (c *clusterDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {

	var data ClusterDataSourceModel
	diags := request.Config.Get(ctx, &data)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	cluster, _, err := c.client.ClusterApi.DetailCluster(ctx, data.ID.ValueInt32())
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster detail",
			"Could not read Cluster detail, unexpected error: "+err.Error())
		return
	}

	data.Name = types.StringValue(cluster.Name)
	data.Status = types.StringValue(cluster.Status)
	data.Version = types.StringValue(cluster.Version)
	data.Endpoint = types.StringValue(cluster.ApiAddress)
	data.VpcConfig = &VpcConfigBlock{
		VpcId: types.Int32Value(cluster.VpcConfig.VpcId),
	}

	securityGroupIds, diags := types.ListValueFrom(ctx, types.Int32Type, cluster.VpcConfig.SecurityGroupIds)
	if diags.HasError() {
		response.Diagnostics.Append(diags...)
		return
	} else {
		data.VpcConfig.SecurityGroupIds = securityGroupIds
	}

	subnetIds, diags := types.ListValueFrom(ctx, types.Int32Type, cluster.VpcConfig.SubnetIds)
	if diags.HasError() {
		response.Diagnostics.Append(diags...)
		return
	} else {
		data.VpcConfig.SubnetIds = subnetIds
	}

	nfs, _, err := c.client.NFSApi.DetailNfsStorage(ctx, voks.BaseResourceReq{
		ClusterId: cluster.Id,
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster NFS detail",
			"Could not read Cluster NFS detail, unexpected error: "+err.Error())
		return
	}

	data.Nfs = &NfsBlock{
		Cpu:              types.Float64Value(nfs.CpuSize),
		Memory:           types.Float64Value(nfs.MemorySize),
		TotalStorageSize: types.Float64Value(nfs.StorageSize),
		Status:           types.StringValue(nfs.Status),
		IpAddress:        types.StringValue(nfs.InternalIp),
	}

	diags = response.State.Set(ctx, &data)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
