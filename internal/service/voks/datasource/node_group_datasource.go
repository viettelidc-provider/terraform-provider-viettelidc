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

type NodeGroupDatasource struct {
	client *voks.APIClient
}

type NodeGroupDataSourceModel struct {
	ID            types.Int32             `tfsdk:"id"`
	ClusterId     types.Int32             `tfsdk:"cluster_id"`
	Name          types.String            `tfsdk:"name"`
	ResourceType  types.String            `tfsdk:"resource_type"`
	AutoRepair    types.Bool              `tfsdk:"auto_repair"`
	ScalingConfig *ScalingConfigBlock     `tfsdk:"scaling_config"`
	Labels        map[string]types.String `tfsdk:"labels"`
	Taint         []TaintConfigBlock      `tfsdk:"taint"`
	Status        types.String            `tfsdk:"status"`
}

type ScalingConfigBlock struct {
	EnableAutoScale types.Bool  `tfsdk:"enable_auto_scale"`
	MaxNode         types.Int32 `tfsdk:"max_node"`
	MinNode         types.Int32 `tfsdk:"min_node"`
}

type TaintConfigBlock struct {
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
	Effect types.String `tfsdk:"effect"`
}

var (
	_ datasource.DataSource              = &NodeGroupDatasource{}
	_ datasource.DataSourceWithConfigure = &NodeGroupDatasource{}
)

func NewNodeGroupDatasource() datasource.DataSource {
	return &NodeGroupDatasource{}
}

func (n *NodeGroupDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

	n.client = voks.NewAPIClient(*cfg)
}

func (n *NodeGroupDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_node_group"
}

func (n *NodeGroupDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Retrieve information about a Node Group associated with a vOKS cluster Id",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int32Attribute{
				Description: "Id of the Node Group.",
				Required:    true,
			},
			"cluster_id": schema.Int32Attribute{
				Description: "The ID of the Cluster into which you want to create one or more Node Groups.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the Node Group.",
				Computed:    true,
			},
			"resource_type": schema.StringAttribute{
				Description: "Instance type associated with the Node Group.",
				Computed:    true,
			},
			"auto_repair": schema.BoolAttribute{
				Description: "Default to `false`. Set it to `true` help keep the nodes in your cluster in a healthy, running state.",
				Computed:    true,
			},
			"labels": schema.MapAttribute{
				Description: "Key/value pairs attached to objects like Pods. They specify identifying attributes meaningfull to users but do not imply semantics to the core system.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of Node Group. Valid values: `CREATING`, `UPDATING`, `SUCCESS`, `ERROR`.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"scaling_config": schema.SingleNestedBlock{
				Description: "Configuration required by the cluster autoscaler to adjust the size of the node group based on current cluster usage.",
				Attributes: map[string]schema.Attribute{
					"enable_auto_scale": schema.BoolAttribute{
						Description: "Default to `false`. Set it to `true` can scale automatically.",
						Computed:    true,
					},
					"max_node": schema.Int32Attribute{
						Description: "Maximum number of nodes in the Node Group. `max_size` need to be greater than or equal 1 and less than or equal to 10 and greater than or equal to `min_size`.",
						Computed:    true,
					},
					"min_node": schema.Int32Attribute{
						Description: "Minimum number of nodes in the Node Group. `min_size` need to be greater than or equal 1 and less than or equal to 10 and less than or equal to `max_size`.",
						Computed:    true,
					},
				},
			},
			"taint": schema.ListNestedBlock{
				Description: "The taints to be applied to the nodes in the Node Group.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Description: "The key for the taint. Must be be 63 characters or less, using letters (a-z, A-Z), numbers (0-9), hyphen (-), underscores (_), and periods (.). Must start and end with a letter, number, or underscore.",
							Computed:    true,
						},
						"value": schema.StringAttribute{
							Description: "The value for the taint. Must be be 63 characters or less, using letters (a-z, A-Z), numbers (0-9), hyphen (-), underscores (_), and periods (.). Must start and end with a letter, number, or underscore",
							Computed:    true,
						},
						"effect": schema.StringAttribute{
							Description: "The effect of the taint, Valid values: `NoSchedule`, `NoExecute`, `PreferNoSchedule`.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (n *NodeGroupDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {

	var data NodeGroupDataSourceModel
	diags := request.Config.Get(ctx, &data)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	detail, _, err := n.client.NodeGroupApi.DetailNodeGroup(ctx, data.ClusterId.ValueInt32(), data.ID.ValueInt32())
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster Node Group detail",
			"Could not read Cluster Addon detail, unexpected error: "+err.Error())
		return
	}

	data.Name = types.StringValue(detail.Name)
	data.ResourceType = types.StringValue(detail.ResourceType)
	data.AutoRepair = types.BoolValue(detail.IsAutoRepair)
	data.ScalingConfig = &ScalingConfigBlock{
		EnableAutoScale: types.BoolValue(detail.IsAutoScale),
		MaxNode:         types.Int32Value(detail.MaxNode),
		MinNode:         types.Int32Value(detail.MinNode),
	}
	data.Status = types.StringValue(detail.Status)

	if len(detail.Labels) > 0 {
		data.Labels = make(map[string]types.String)
		for _, label := range detail.Labels {
			data.Labels[label.Key] = types.StringValue(label.Value)
		}
	}

	if len(detail.Taints) > 0 {
		data.Taint = make([]TaintConfigBlock, 0)
		for _, taint := range detail.Taints {
			data.Taint = append(data.Taint, TaintConfigBlock{
				Key:    types.StringValue(taint.Key),
				Value:  types.StringValue(taint.Value),
				Effect: types.StringValue(taint.Effect),
			})
		}
	}

	diags = response.State.Set(ctx, &data)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
