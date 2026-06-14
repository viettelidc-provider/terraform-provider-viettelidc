package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ datasource.DataSource              = (*FloatingIPDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*FloatingIPDataSource)(nil)
)

// FloatingIPDataSource implements `data "viettelidc_floating_ip"` (lookup by id).
type FloatingIPDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type FloatingIPDataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	PublicIP           types.String `tfsdk:"public_ip"`
	InstanceID         types.String `tfsdk:"instance_id"`
	NetworkInterfaceID types.String `tfsdk:"network_interface_id"`
	VpcID              types.String `tfsdk:"vpc_id"`
}

func NewFloatingIPDataSource() datasource.DataSource { return &FloatingIPDataSource{} }

func (d *FloatingIPDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_floating_ip"
}

func (d *FloatingIPDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ViettelIDC Floating IP by its ID (vttFloatingId).",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Required: true, Description: "Floating IP ID (vttFloatingId)."},
			"public_ip":            schema.StringAttribute{Computed: true, Description: "Public IPv4 address."},
			"instance_id":          schema.StringAttribute{Computed: true, Description: "Associated VM instance ID."},
			"network_interface_id": schema.StringAttribute{Computed: true, Description: "Associated NIC ID."},
			"vpc_id":               schema.StringAttribute{Optional: true, Computed: true, Description: "VPC ID."},
		},
	}
}

func (d *FloatingIPDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *FloatingIPDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg FloatingIPDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing required attribute", "'id' is required.")
		return
	}

	vpcID := cfg.VpcID.ValueString()
	if vpcID == "" {
		vpcID = d.defaultVpcID
	}

	body := map[string]interface{}{
		"floating_ip_id": cfg.ID.ValueString(),
		"vpc_id":         vpcID,
		"customer_id":    d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathFloatingIPDetail, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := mapFloatingIPDataSourceResponse(apiResp, &cfg); err != nil {
		resp.Diagnostics.AddError("Floating IP response decode failed", err.Error())
		return
	}
	cfg.VpcID = types.StringValue(vpcID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

func mapFloatingIPDataSourceResponse(resp *client.APIResponse, m *FloatingIPDataSourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	if ip := asString(data, "publicIp"); ip != "" {
		m.PublicIP = types.StringValue(ip)
	} else if ip := asString(data, "publicIP"); ip != "" {
		m.PublicIP = types.StringValue(ip)
	}
	if vmID := asIDString(data, "vttVmId"); vmID != "" {
		m.InstanceID = types.StringValue(vmID)
	}
	if nicID := asString(data, "vttNetworkInterfaceId"); nicID != "" {
		m.NetworkInterfaceID = types.StringValue(nicID)
	}
	return nil
}
