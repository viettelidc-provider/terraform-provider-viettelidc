package networking

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ datasource.DataSource              = (*NetworkInterfacesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*NetworkInterfacesDataSource)(nil)
)

// NetworkInterfacesDataSource implements `data "viettelidc_network_interfaces"`.
type NetworkInterfacesDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type NetworkInterfacesDataSourceModel struct {
	VpcID             types.String           `tfsdk:"vpc_id"`
	Filters           *NicFilters            `tfsdk:"filters"`
	NetworkInterfaces []NetworkInterfaceItem `tfsdk:"network_interfaces"`
}

type NicFilters struct {
	Name     types.String `tfsdk:"name"`
	SubnetID types.String `tfsdk:"subnet_id"`
	Status   types.String `tfsdk:"status"`
}

type NetworkInterfaceItem struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	SubnetID     types.String `tfsdk:"subnet_id"`
	IpAssignType types.String `tfsdk:"ip_assign_type"`
	IpAddress    types.String `tfsdk:"ip_address"`
	Description  types.String `tfsdk:"description"`
	Status       types.String `tfsdk:"status"`
}

func NewNetworkInterfacesDataSource() datasource.DataSource { return &NetworkInterfacesDataSource{} }

func (d *NetworkInterfacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_network_interfaces"
}

func (d *NetworkInterfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List ViettelIDC NICs in a VPC with optional filters (client-side).",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{Optional: true},
			"filters": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"name":      schema.StringAttribute{Optional: true},
					"subnet_id": schema.StringAttribute{Optional: true},
					"status":    schema.StringAttribute{Optional: true},
				},
			},
			"network_interfaces": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true},
						"name":           schema.StringAttribute{Computed: true},
						"subnet_id":      schema.StringAttribute{Computed: true},
						"ip_assign_type": schema.StringAttribute{Computed: true},
						"ip_address":     schema.StringAttribute{Computed: true},
						"description":    schema.StringAttribute{Computed: true},
						"status":         schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *NetworkInterfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *NetworkInterfacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg NetworkInterfacesDataSourceModel
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
	apiResp, diags := callAPI(ctx, d.client, pathNicList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := decodeNicList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode NIC list", err.Error())
		return
	}

	totalReturned := len(items)
	items = applyNicFilters(items, cfg.Filters)

	cfg.VpcID = types.StringValue(vpcID)
	cfg.NetworkInterfaces = make([]NetworkInterfaceItem, 0, len(items))
	for _, raw := range items {
		cfg.NetworkInterfaces = append(cfg.NetworkInterfaces, NetworkInterfaceItem{
			ID:           types.StringValue(asString(raw, "vttNetworkInterfaceId")),
			Name:         types.StringValue(asString(raw, "name")),
			SubnetID:     types.StringValue(asString(raw, "vttSubnetId")),
			IpAssignType: types.StringValue(asString(raw, "ipAssignType")),
			IpAddress:    types.StringValue(asString(raw, "ipAddress")),
			Description:  types.StringValue(asString(raw, "description")),
			Status:       types.StringValue(asString(raw, "status")),
		})
	}

	if totalReturned >= listWarningThreshold {
		resp.Diagnostics.AddWarning(
			"NIC list may be truncated",
			"Returned 1000 or more NICs from CSA before client-side filtering; pagination may be applied. Consider narrowing by vpc_id.",
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// applyNicFilters filters NIC list maps by name/subnet_id/status (exact match).
func applyNicFilters(items []map[string]interface{}, f *NicFilters) []map[string]interface{} {
	if f == nil {
		return items
	}
	wantName := f.Name.ValueString()
	wantSubnet := f.SubnetID.ValueString()
	wantStatus := f.Status.ValueString()
	if wantName == "" && wantSubnet == "" && wantStatus == "" {
		return items
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if wantName != "" && asString(item, "name") != wantName {
			continue
		}
		if wantSubnet != "" && asString(item, "vttSubnetId") != wantSubnet {
			continue
		}
		if wantStatus != "" && asString(item, "status") != wantStatus {
			continue
		}
		out = append(out, item)
	}
	return out
}

// Compile-time client reference (avoid unused import if pruned later).
var _ = (*client.Client)(nil)
