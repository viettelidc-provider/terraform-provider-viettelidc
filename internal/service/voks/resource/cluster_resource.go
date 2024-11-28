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
	_ resource.Resource                = &clusterResource{}
	_ resource.ResourceWithConfigure   = &clusterResource{}
	_ resource.ResourceWithImportState = &clusterResource{}
)

type clusterResource struct {
	client *voks.APIClient
}

type ClusterResourceModel struct {
	ID        types.Int32     `tfsdk:"id"`
	Name      types.String    `tfsdk:"name"`
	Status    types.String    `tfsdk:"status"`
	Version   types.String    `tfsdk:"version"`
	Endpoint  types.String    `tfsdk:"endpoint"`
	Nfs       *NfsBlock       `tfsdk:"nfs"`
	VpcConfig *VpcConfigBlock `tfsdk:"vpc_config"`
}

type VpcConfigBlock struct {
	VpcId types.Int32 `tfsdk:"vpc_id"`
	//SecurityGroupIds types.List  `tfsdk:"security_group_ids"`
	//SubnetIds        types.List  `tfsdk:"subnet_ids"`
}

type NfsBlock struct {
	Cpu                   types.Float64 `tfsdk:"cpu"`
	Memory                types.Float64 `tfsdk:"memory"`
	TotalStorageSize      types.Float64 `tfsdk:"total_storage_size"`
	AdditionalStorageSize types.Int32   `tfsdk:"additional_storage_size"`
	Status                types.String  `tfsdk:"status"`
	IpAddress             types.String  `tfsdk:"ip_address"`
}

func NewClusterResource() resource.Resource {
	return &clusterResource{}
}

func (c *clusterResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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

func (c *clusterResource) Metadata(ctx context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_cluster"
}

func (c *clusterResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	//Generate fully response schema from ClusterResourceModel
	response.Schema = schema.Schema{
		Description: "Manage a Kubernetes Cluster resource within ViettelIdc.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int32Attribute{
				Description: "Id of the Cluster.",
				Computed: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the Cluster.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "The current status of Cluster. Valid values: `POWER_ON`, `POWER_OFF`, `ERROR`.",
				Computed: true,
			},
			"version": schema.StringAttribute{
				Description: "Version of Cluster.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"endpoint": schema.StringAttribute{
				Description: "Endpoint for your API server.",
				Computed: true,
			},
			"nfs": schema.SingleNestedAttribute{
				Description: "NFS storage enables multiple nodes in the cluster to access the same file system over a network.",
				Optional: true,
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"cpu": schema.Float64Attribute{
						Description: "The CPU size of NFS server.",
						Computed: true,
					},
					"memory": schema.Float64Attribute{
						Description: "The memory size of NFS server.",
						Computed: true,
					},
					"total_storage_size": schema.Float64Attribute{
						Description: "The size allocated for NFS volumes.",
						Computed: true,
					},
					"additional_storage_size": schema.Int32Attribute{
						Description: "The additional storage allocated for NFS volumes.",
						Optional: true,
					},
					"status": schema.StringAttribute{
						Description: "Status of Cluster NFS Storage. When the NFS Storage is present in Terraform, its status will always be `POWER_ON`. Valid values: `POWER_ON`, `UPDATING`, `ERROR`.",
						Computed: true,
					},
					"ip_address": schema.StringAttribute{
						Description: "Internal IP of NFS server that can be accessed by internal network of your Cluster.",
						Computed: true,
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
						Required: true,
						PlanModifiers: []planmodifier.Int32{
							int32planmodifier.RequiresReplace(),
						},
					},
					//"security_group_ids": schema.ListAttribute{
					//	Computed:    true,
					//	ElementType: types.Int32Type,
					//	PlanModifiers: []planmodifier.List{
					//		listplanmodifier.UseStateForUnknown(),
					//	},
					//},
					//"subnet_ids": schema.ListAttribute{
					//	Computed:    true,
					//	ElementType: types.Int32Type,
					//	PlanModifiers: []planmodifier.List{
					//		listplanmodifier.UseStateForUnknown(),
					//	},
					//},
				},
			},
		},
	}
}

func (c *clusterResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	response.Diagnostics.AddWarning(
		"Error creating Cluster",
		"Could not create Cluster, this action is not supported.")
	return

	//var state ClusterResourceModel
	//
	//cluster, _, err := c.client.ClusterApi.DetailCluster(ctx, voks.BaseResourceReq{
	//	ClusterId: 2459,
	//})
	//if err != nil {
	//	response.Diagnostics.AddError(
	//		"Error reading Cluster detail",
	//		"Could not read Cluster detail, unexpected error: "+err.Error())
	//	return
	//}
	//
	//state.ID = types.Int32Value(cluster.Id)
	//state.Name = types.StringValue(cluster.Name)
	//state.Status = types.StringValue(cluster.Status)
	//state.Version = types.StringValue(cluster.Version)
	//state.Endpoint = types.StringValue(cluster.ApiAddress)
	//state.VpcConfig = &VpcConfigBlock{
	//	VpcId: types.Int32Value(cluster.VpcId),
	//}
	//
	//nfs, _, err := c.client.NFSApi.DetailNfsStorage(ctx, voks.BaseResourceReq{
	//	ClusterId: cluster.Id,
	//})
	//if err != nil {
	//	response.Diagnostics.AddError(
	//		"Error reading Cluster NFS detail",
	//		"Could not read Cluster NFS detail, unexpected error: "+err.Error())
	//	return
	//}
	//
	//state.Nfs = &NfsBlock{
	//	Cpu:              types.Float64Value(nfs.CpuSize),
	//	Memory:           types.Float64Value(nfs.MemorySize),
	//	TotalStorageSize: types.Float64Value(nfs.StorageSize),
	//	Status:           types.StringValue(nfs.Status),
	//	IpAddress:        types.StringValue(nfs.InternalIp),
	//}
	//
	//diags := response.State.Set(ctx, &state)
	//response.Diagnostics.Append(diags...)
	//if response.Diagnostics.HasError() {
	//	return
	//}
}

func (c *clusterResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {

	var state ClusterResourceModel
	diags := request.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	cluster, _, err := c.client.ClusterApi.DetailCluster(ctx, voks.BaseResourceReq{
		ClusterId: state.ID.ValueInt32(),
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster detail",
			"Could not read Cluster detail, unexpected error: "+err.Error())
		return
	}

	state.ID = types.Int32Value(cluster.Id)
	state.Name = types.StringValue(cluster.Name)
	state.Status = types.StringValue(cluster.Status)
	state.Version = types.StringValue(cluster.Version)
	state.Endpoint = types.StringValue(cluster.ApiAddress)
	state.VpcConfig = &VpcConfigBlock{
		VpcId: types.Int32Value(cluster.VpcId),
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

	state.Nfs = &NfsBlock{
		Cpu:              types.Float64Value(nfs.CpuSize),
		Memory:           types.Float64Value(nfs.MemorySize),
		TotalStorageSize: types.Float64Value(nfs.StorageSize),
		Status:           types.StringValue(nfs.Status),
		IpAddress:        types.StringValue(nfs.InternalIp),
	}

	diags = response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}

func (c *clusterResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {

	id, err := strconv.ParseInt(request.ID, 10, 32)
	if err != nil {
		response.Diagnostics.AddError(
			"Error parsing Cluster ID",
			"Could not parse Cluster ID, unexpected error: "+err.Error())
		return
	}

	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (c *clusterResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {

	var state, plan ClusterResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)

	if state.Name != plan.Name {
		response.Diagnostics.AddError(
			"Error updating Cluster",
			"Could not update field `name` of the Cluster, this action is not supported.")
		return
	}

	if state.Version != plan.Version {
		response.Diagnostics.AddError(
			"Error updating Cluster",
			"Could not update field `version` of the Cluster, this action is not supported.")
		return
	}

	isUpdateNfs := false
	if state.Nfs == nil || state.Nfs.AdditionalStorageSize.IsNull() {
		if plan.Nfs != nil && !plan.Nfs.AdditionalStorageSize.IsNull() {
			if plan.Nfs.AdditionalStorageSize.ValueInt32() < 10 || plan.Nfs.AdditionalStorageSize.ValueInt32() > 2000 {
				response.Diagnostics.AddError(
					"Error updating Cluster",
					"Could not update field `nfs.additional_storage_size` of the Cluster, value must be between 10 and 2000.")
				return
			}
			isUpdateNfs = true
		}
	} else {
		if plan.Nfs == nil || plan.Nfs.AdditionalStorageSize.IsNull() {
			response.Diagnostics.AddError(
				"Error updating Cluster",
				"Could not set field `nfs.additional_storage_size` to null, this action is not supported.")
			return
		}
		if plan.Nfs.AdditionalStorageSize.ValueInt32() < state.Nfs.AdditionalStorageSize.ValueInt32() {
			response.Diagnostics.AddError(
				"Error updating Cluster",
				"Could not update field `nfs.additional_storage_size` of the Cluster, value must be equal or greater than old value.")
			return
		}
		if plan.Nfs.AdditionalStorageSize.ValueInt32() > 2000 {
			response.Diagnostics.AddError(
				"Error updating Cluster",
				"Could not update field `nfs.additional_storage_size` of the Cluster, value must be between 10 and 2000.")
			return
		}
		if plan.Nfs.AdditionalStorageSize.ValueInt32() > state.Nfs.AdditionalStorageSize.ValueInt32() {
			isUpdateNfs = true
		}
	}

	if isUpdateNfs {
		_, err := c.client.NFSApi.ExtendNfsStorage(ctx, voks.AddonNfsRequest{
			ClusterId:     plan.ID.ValueInt32(),
			AddOnsStorage: plan.Nfs.AdditionalStorageSize.ValueInt32(),
		})
		if err != nil {
			response.Diagnostics.AddError(
				"Error updating Cluster",
				"Could not update NFS Storage of Cluster, unexpected error: "+err.Error())
			return
		}
		for {
			nfs, _, err := c.client.NFSApi.DetailNfsStorage(ctx, voks.BaseResourceReq{
				ClusterId: plan.ID.ValueInt32(),
			})
			if err != nil {
				response.Diagnostics.AddError(
					"Error updating Cluster NFS status",
					"Could not read Cluster NFS detail, unexpected error: "+err.Error())
				return
			}
			if strings.EqualFold(nfs.Status, "POWERED_ON") {
				plan.Nfs.Cpu = types.Float64Value(nfs.CpuSize)
				plan.Nfs.Memory = types.Float64Value(nfs.MemorySize)
				plan.Nfs.TotalStorageSize = types.Float64Value(nfs.StorageSize)
				plan.Nfs.Status = types.StringValue(nfs.Status)
				plan.Nfs.IpAddress = types.StringValue(nfs.InternalIp)
				break
			}
			if strings.EqualFold(nfs.Status, "ERROR") {
				response.Diagnostics.AddError(
					"Error extending Cluster NFS Storage",
					"Could not extend Cluster NFS Storage.")
				return
			}
			time.Sleep(10 * time.Second)
		}
	}

	// Update cluster detail
	cluster, _, err := c.client.ClusterApi.DetailCluster(ctx, voks.BaseResourceReq{
		ClusterId: state.ID.ValueInt32(),
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster detail",
			"Could not read Cluster detail, unexpected error: "+err.Error())
		return
	}

	plan.Name = types.StringValue(cluster.Name)
	plan.Status = types.StringValue(cluster.Status)
	plan.Version = types.StringValue(cluster.Version)
	plan.Endpoint = types.StringValue(cluster.ApiAddress)
	plan.VpcConfig = &VpcConfigBlock{
		VpcId: types.Int32Value(cluster.VpcId),
	}

	response.Diagnostics.Append(response.State.Set(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}
}

func (c *clusterResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	response.Diagnostics.AddWarning(
		"Error deleting Cluster",
		"Could not delete Cluster, this action is not supported.")
	return
}
