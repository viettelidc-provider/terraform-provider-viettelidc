// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
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
	_ resource.Resource                = &nodeGroupResource{}
	_ resource.ResourceWithConfigure   = &nodeGroupResource{}
	_ resource.ResourceWithImportState = &nodeGroupResource{}
)

type nodeGroupResource struct {
	client *voks.APIClient
}

func NewNodeGroupResource() resource.Resource {
	return &nodeGroupResource{}
}

type NodeGroupResourceModel struct {
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

func (n *nodeGroupResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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

func (n *nodeGroupResource) Metadata(ctx context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_node_group"
}

func (n *nodeGroupResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Collection of nodes in a ViettelIdc Kubernetes cluster that share similar configuration, typically based on their hardware, instance type.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int32Attribute{
				Description: "Id of the VettelIdc Kuberneters Cluster.",
				Computed: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.Int32Attribute{
				Description: "The ID of the Cluster into which you want to create one or more node groups.",
				Required: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the VettelIdc Kuberneters Cluster.",
				Required: true,
			},
			"resource_type": schema.StringAttribute{
				Description: "Instance type associated with the ViettelIdc Node Group.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"auto_repair": schema.BoolAttribute{
				Description: "Default to `false`. Set it to `true` help keep the nodes in your ViettelIdc Kubernetes cluster in a healthy, running state.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"labels": schema.MapAttribute{
				Description: "Key/value pairs attached to objects like Pods. They specify identifying attributes meaningfull to users but do not imply semantics to the core system.",
				ElementType: types.StringType,
				Optional:    true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "The current status of ViettelIdc Kubernetes Node Group. Valid values: `CREATING`, `UPDATING`, `SUCCESS`, `ERROR`.",
				Computed: true,
			},
		},
		Blocks: map[string]schema.Block{			
			"scaling_config": schema.SingleNestedBlock{
				Description: "Configuration required by the cluster autoscaler to adjust the size of the node group based on current cluster usage.",
				Attributes: map[string]schema.Attribute{
					"enable_auto_scale": schema.BoolAttribute{
						Description: "Default to `false`. Set it to `true` can scale automatically.",
						Required: true,
					},
					"max_node": schema.Int32Attribute{
						Description: "Maximum number of nodes in the Node Group. `max_size` need to be greater than or equal 1 and less than or equal to 10 and greater than or equal to `min_size`.",
						Required: true,
					},
					"min_node": schema.Int32Attribute{
						Description: "Minimum number of nodes in the Node Group. `min_size` need to be greater than or equal 1 and less than or equal to 10 and less than or equal to `max_size`.",
						Required: true,
					},
				},
			},
			"taint": schema.ListNestedBlock{
				Description: "The Kubernetes taints to be applied to the nodes in the Node Group.",
				NestedObject: schema.NestedBlockObject{					
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Description: "The key for the taint. Must be be 63 characters or less, using letters (a-z, A-Z), numbers (0-9), hyphen (-), underscores (_), and periods (.). Must start and end with a letter, number, or underscore.",
							Required: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.RequiresReplace(),
							},
						},
						"value": schema.StringAttribute{
							Description: "The value for the taint. Must be be 63 characters or less, using letters (a-z, A-Z), numbers (0-9), hyphen (-), underscores (_), and periods (.). Must start and end with a letter, number, or underscore",
							Required: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.RequiresReplace(),
							},
						},
						"effect": schema.StringAttribute{
							Description: "The effect of the taint, Valid values: `NoSchedule`, `NoExecute`, `PreferNoSchedule`.",
							Required: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.RequiresReplace(),
							},
						},
					},
				},
			},
		},
	}
}

func (n *nodeGroupResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {

	var plan NodeGroupResourceModel
	diags := request.Plan.Get(ctx, &plan)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	reqBody := voks.CreateNodeGroupRequest{
		ClusterId:    plan.ClusterId.ValueInt32(),
		Name:         plan.Name.ValueString(),
		ResourceType: plan.ResourceType.ValueString(),
	}

	errSum, errDetail := scalingConfigValidator(plan.ScalingConfig)
	if errSum != "" && errDetail != "" {
		response.Diagnostics.AddError(errSum, errDetail)
		return
	}
	reqBody.IsAutoScale = plan.ScalingConfig.EnableAutoScale.ValueBool()
	reqBody.MinNode = plan.ScalingConfig.MinNode.ValueInt32()
	reqBody.MaxNode = plan.ScalingConfig.MaxNode.ValueInt32()

	if !plan.AutoRepair.IsNull() && plan.AutoRepair.ValueBool() {
		response.Diagnostics.AddError(
			"Invalid Configuration",
			"`auto_repair` must be set to `false` when creating a node group.",
		)
		return
	}

	for key, value := range plan.Labels {
		reqBody.Labels = append(reqBody.Labels, voks.NodeGroupLabel{
			Key:   key,
			Value: value.ValueString(),
		})
	}

	for _, element := range plan.Taint {
		reqBody.Taints = append(reqBody.Taints, voks.NodeGroupTaint{
			Key:    element.Key.ValueString(),
			Value:  element.Value.ValueString(),
			Effect: element.Effect.ValueString(),
		})
	}

	resBody, _, err := n.client.NodeGroupApi.CreateNodeGroup(ctx, reqBody)
	if err != nil {
		response.Diagnostics.AddError(
			"Error creating Cluster Node Group",
			"Could not create Cluster Node Group, unexpected error: "+err.Error())
		return
	}

	for {
		detail, _, err := n.client.NodeGroupApi.DetailNodeGroup(ctx, voks.DetailNodeGroupRequest{
			Id:        resBody.Id,
			ClusterId: resBody.ClusterId,
		})
		if err != nil {
			response.Diagnostics.AddError(
				"Error updating Cluster Node Group status",
				"Could not update Cluster Node Group status, unexpected error: "+err.Error())
			return
		}
		if detail.Status == "success" {
			plan.ID = types.Int32Value(detail.Id)
			plan.Status = types.StringValue(detail.Status)
			break
		}
		if detail.Status == "error" {
			response.Diagnostics.AddError(
				"Error creating Cluster Node Group",
				"Could not create Cluster Node Group.")
			return
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

func (n *nodeGroupResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	idParts := strings.Split(request.ID, ",")

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		response.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: id,cluster_id. Got: %q", request.ID),
		)
		return
	}

	id, err := strconv.ParseInt(idParts[0], 10, 32)
	if err != nil {
		response.Diagnostics.AddError(
			"Error parsing Cluster Node Group ID",
			"Could not parse Cluster Node Group ID, unexpected error: "+err.Error())
		return
	}

	clusterId, err := strconv.ParseInt(idParts[1], 10, 32)
	if err != nil {
		response.Diagnostics.AddError(
			"Error parsing Cluster ID",
			"Could not parse Cluster ID, unexpected error: "+err.Error())
		return
	}

	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("id"), id)...)
	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("cluster_id"), clusterId)...)
}

func (n *nodeGroupResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {

	var state NodeGroupResourceModel
	diags := request.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	detail, _, err := n.client.NodeGroupApi.DetailNodeGroup(ctx, voks.DetailNodeGroupRequest{
		ClusterId: state.ClusterId.ValueInt32(),
		Id:        state.ID.ValueInt32(),
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster Node Group detail",
			"Could not read Cluster Addon detail, unexpected error: "+err.Error())
		return
	}

	state.Name = types.StringValue(detail.Name)
	state.ResourceType = types.StringValue(detail.ResourceType)
	state.AutoRepair = types.BoolValue(detail.IsAutoRepair)
	state.ScalingConfig = &ScalingConfigBlock{
		EnableAutoScale: types.BoolValue(detail.IsAutoScale),
		MaxNode:         types.Int32Value(detail.MaxNode),
		MinNode:         types.Int32Value(detail.MinNode),
	}
	state.Status = types.StringValue(detail.Status)

	if len(detail.Labels) > 0 {
		state.Labels = make(map[string]types.String)
		for _, label := range detail.Labels {
			state.Labels[label.Key] = types.StringValue(label.Value)
		}
	}

	if len(detail.Taints) > 0 {
		state.Taint = make([]TaintConfigBlock, 0)
		for _, taint := range detail.Taints {
			state.Taint = append(state.Taint, TaintConfigBlock{
				Key:    types.StringValue(taint.Key),
				Value:  types.StringValue(taint.Value),
				Effect: types.StringValue(taint.Effect),
			})
		}
	}

	diags = response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}

func (n *nodeGroupResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {

	var plan NodeGroupResourceModel
	diags := request.Plan.Get(ctx, &plan)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	reqBody := voks.UpdateNodeGroupRequest{
		ClusterId:    plan.ClusterId.ValueInt32(),
		Id:           plan.ID.ValueInt32(),
		Name:         plan.Name.ValueString(),
		IsAutoRepair: plan.AutoRepair.ValueBool(),
	}

	errSum, errDetail := scalingConfigValidator(plan.ScalingConfig)
	if errSum != "" && errDetail != "" {
		response.Diagnostics.AddError(errSum, errDetail)
		return
	}
	reqBody.IsAutoScale = plan.ScalingConfig.EnableAutoScale.ValueBool()
	reqBody.MinNode = plan.ScalingConfig.MinNode.ValueInt32()
	reqBody.MaxNode = plan.ScalingConfig.MaxNode.ValueInt32()

	updateRes, _, err := n.client.NodeGroupApi.UpdateNodeGroup(ctx, reqBody)
	if err != nil {
		response.Diagnostics.AddError(
			"Error updating Cluster Node Group",
			"Could not update Cluster Node Group, unexpected error: "+err.Error())
		return
	}

	for {
		//refresh status util is success
		detailRes, _, err := n.client.NodeGroupApi.DetailNodeGroup(ctx, voks.DetailNodeGroupRequest{
			ClusterId: updateRes.ClusterId,
			Id:        updateRes.Id,
		})
		if err != nil {
			response.Diagnostics.AddError(
				"Error updating Cluster Node Group status",
				"Could not update Cluster Node Group status, unexpected error: "+err.Error())
			return
		}
		if detailRes.Status == "success" {
			// Update plan with new data
			plan.AutoRepair = types.BoolValue(detailRes.IsAutoRepair)
			plan.ScalingConfig.EnableAutoScale = types.BoolValue(detailRes.IsAutoScale)
			plan.ScalingConfig.MinNode = types.Int32Value(detailRes.MinNode)
			plan.ScalingConfig.MaxNode = types.Int32Value(detailRes.MaxNode)
			plan.Status = types.StringValue(detailRes.Status)
			break
		}
		if detailRes.Status == "error" {
			response.Diagnostics.AddError(
				"Error updating Cluster Node Group",
				"Could not update Cluster Node Group.")
			return
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

func (n *nodeGroupResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {

	var state NodeGroupResourceModel
	diags := request.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	_, err := n.client.NodeGroupApi.DeleteNodeGroup(ctx, voks.DeleteNodeGroupRequest{
		ClusterId: state.ClusterId.ValueInt32(),
		Id:        state.ID.ValueInt32(),
	})

	if err != nil {
		response.Diagnostics.AddError(
			"Error deleting Cluster Node Group",
			"Could not delete Cluster Node Group, unexpected error: "+err.Error())
		return
	}

	// Check status of cluster
	for {
		detailCluster, _, err := n.client.ClusterApi.DetailCluster(ctx, voks.BaseResourceReq{
			ClusterId: state.ClusterId.ValueInt32(),
		})
		if err != nil {
			response.Diagnostics.AddError(
				"Error updating Cluster status",
				"Could not update Cluster status, unexpected error: "+err.Error())
			return
		}
		if detailCluster.Status == "error" {
			// Return error response: cluster got err please contact tech support
			response.Diagnostics.AddError(
				"Error deleting Cluster Node Group",
				"Could not delete Cluster Node Group, Cluster got ERROR status, please contact Tech Support.")
			return
		}
		if detailCluster.Status == "success" {
			break
		}
		time.Sleep(10 * time.Second)
	}

	// Check status of deleted node group
	detailNodeGroup, _, err := n.client.NodeGroupApi.DetailNodeGroup(ctx, voks.DetailNodeGroupRequest{
		ClusterId: state.ClusterId.ValueInt32(),
		Id:        state.ClusterId.ValueInt32(),
	})
	if err != nil {
		// If node group is not found, it means it was deleted successfully
	} else {
		if detailNodeGroup.Status == "error" {
			response.Diagnostics.AddWarning(
				"Error deleting Cluster Node Group",
				"Could not delete Cluster Node Group, Node Group got ERROR status, please contact Tech Support.")
		}
	}
}

func scalingConfigValidator(scalingCfg *ScalingConfigBlock) (errorSummary, errorDetail string) {
	if scalingCfg == nil {
		return "Invalid Configuration", "`scaling_config` must be set"
	} else {
		if scalingCfg.EnableAutoScale.ValueBool() {
			if minNode, maxNode := scalingCfg.MinNode.ValueInt32(), scalingCfg.MaxNode.ValueInt32(); maxNode <= minNode || maxNode > 10 {
				return "Invalid Configuration", fmt.Sprintf(
					"`max_node` must be greater than `min_node` and less than or equal to 10. Got: min_node=%d, max_node=%d",
					minNode, maxNode)

			}
			if minNode, maxNode := scalingCfg.MinNode.ValueInt32(), scalingCfg.MaxNode.ValueInt32(); minNode <= 0 || minNode >= maxNode {
				return "Invalid Configuration", fmt.Sprintf(
					"`min_node` must be greater than 0 and less than `max_node`. Got: min_node=%d, max_node=%d",
					minNode, maxNode)
			}
		} else {
			if minNode, maxNode := scalingCfg.MinNode.ValueInt32(), scalingCfg.MaxNode.ValueInt32(); minNode != 1 || maxNode != 1 {
				return "Invalid Configuration", fmt.Sprintf(
					"`min_node` and `max_node` must be set to 1 when `enable_auto_scale` is false. Got: min_node=%d, max_node=%d",
					minNode, maxNode)
			}
		}
	}
	return "", ""
}
