package vpc

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

// Compile-time interface assertions.
var (
	_ datasource.DataSource              = (*AutoscaleGroupsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*AutoscaleGroupsDataSource)(nil)
)

// AutoscaleGroupsDataSource implements `data "viettelidc_autoscale_groups"` (list all).
// NOTE: There is no singular autoscale_group data source — the API has no detail endpoint.
type AutoscaleGroupsDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// AutoscaleGroupItem is one element in the list data source result.
type AutoscaleGroupItem struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	LaunchTemplateID  types.String `tfsdk:"launch_template_id"`
	IsAutoscale       types.Bool   `tfsdk:"is_autoscale"`
	DesiredCapacity   types.Int64  `tfsdk:"desired_capacity"`
	MinSize           types.Int64  `tfsdk:"min_size"`
	MaxSize           types.Int64  `tfsdk:"max_size"`
	MetricType        types.String `tfsdk:"metric_type"`
	ScaleOutThreshold types.Int64  `tfsdk:"scale_out_threshold"`
	ScaleInThreshold  types.Int64  `tfsdk:"scale_in_threshold"`
	HasLoadBalancer   types.Bool   `tfsdk:"has_load_balancer"`
}

// AutoscaleGroupsDataSourceModel is the schema model for the list data source.
type AutoscaleGroupsDataSourceModel struct {
	VpcID          types.String         `tfsdk:"vpc_id"`
	AutoscaleGroups []AutoscaleGroupItem `tfsdk:"autoscale_groups"`
}

// NewAutoscaleGroupsDataSource constructs the plural data source.
func NewAutoscaleGroupsDataSource() datasource.DataSource { return &AutoscaleGroupsDataSource{} }

func (d *AutoscaleGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_autoscale_groups"
}

func (d *AutoscaleGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	itemAttrs := map[string]schema.Attribute{
		"id":                  schema.StringAttribute{Computed: true, Description: "Autoscale Group ID."},
		"name":                schema.StringAttribute{Computed: true, Description: "Group name."},
		"launch_template_id":  schema.StringAttribute{Computed: true, Description: "Launch Template ID."},
		"is_autoscale":        schema.BoolAttribute{Computed: true, Description: "Whether auto-scaling is enabled."},
		"desired_capacity":    schema.Int64Attribute{Computed: true, Description: "Desired instance count."},
		"min_size":            schema.Int64Attribute{Computed: true, Description: "Minimum instance count."},
		"max_size":            schema.Int64Attribute{Computed: true, Description: "Maximum instance count."},
		"metric_type":         schema.StringAttribute{Computed: true, Description: "Scaling metric type."},
		"scale_out_threshold": schema.Int64Attribute{Computed: true, Description: "Scale-out CPU % threshold."},
		"scale_in_threshold":  schema.Int64Attribute{Computed: true, Description: "Scale-in CPU % threshold."},
		"has_load_balancer":   schema.BoolAttribute{Computed: true, Description: "Whether attached to a Load Balancer."},
	}
	resp.Schema = schema.Schema{
		Description: "List all ViettelIDC Autoscale Groups in a VPC.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{Optional: true, Computed: true, Description: "VPC filter; falls back to provider default."},
			"autoscale_groups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: itemAttrs,
				},
			},
		},
	}
}

func (d *AutoscaleGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *AutoscaleGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg AutoscaleGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathAutoscaleGroupList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeAutoscaleGroupList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Decode autoscale group list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.AutoscaleGroups = make([]AutoscaleGroupItem, 0, len(items))
	for _, raw := range items {
		item := AutoscaleGroupItem{
			ID:                types.StringValue(asIDString(raw, "id")),
			Name:              types.StringValue(asString(raw, "name")),
			LaunchTemplateID:  types.StringValue(asIDString(raw, "launchTemplateId")),
			IsAutoscale:       types.BoolValue(asBool(raw, "isAutoscale")),
			DesiredCapacity:   types.Int64Value(asInt64(raw, "desiredCapacity")),
			MinSize:           types.Int64Value(asInt64(raw, "minSize")),
			MaxSize:           types.Int64Value(asInt64(raw, "maxSize")),
			MetricType:        types.StringValue(asString(raw, "metricType")),
			ScaleOutThreshold: types.Int64Value(asInt64(raw, "scaleOutThreshold")),
			ScaleInThreshold:  types.Int64Value(asInt64(raw, "scaleInThreshold")),
			HasLoadBalancer:   types.BoolValue(asBool(raw, "hasLoadBalancer")),
		}
		cfg.AutoscaleGroups = append(cfg.AutoscaleGroups, item)
	}

	if len(cfg.AutoscaleGroups) >= listWarningThreshold {
		resp.Diagnostics.AddWarning(
			"Autoscale group list may be truncated",
			"Returned 1000 or more autoscale groups; the API may have applied a default page size.",
		)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
