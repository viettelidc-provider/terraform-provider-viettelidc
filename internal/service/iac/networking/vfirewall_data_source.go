package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ datasource.DataSource              = (*VFirewallsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VFirewallsDataSource)(nil)
)

// VFirewallsDataSource implements `data "viettelidc_vfirewalls"` — list all
// vFirewall instances in a VPC. Only list is supported by the API.
type VFirewallsDataSource struct {
	client     *client.Client
	customerID string
	defaultVpcID string
}

type VFirewallsDataSourceModel struct {
	VpcID types.String      `tfsdk:"vpc_id"`
	Items []VFirewallItem   `tfsdk:"items"`
}

type VFirewallItem struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Status           types.String `tfsdk:"status"`
	AvailabilityZone types.String `tfsdk:"availability_zone"`
	CPU              types.Int64  `tfsdk:"cpu"`
	Memory           types.Int64  `tfsdk:"memory"`
	ExternalIP       types.String `tfsdk:"external_ip"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

func NewVFirewallsDataSource() datasource.DataSource { return &VFirewallsDataSource{} }

func (d *VFirewallsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_vfirewalls"
}

func (d *VFirewallsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	itemAttrs := map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"status":            types.StringType,
		"availability_zone": types.StringType,
		"cpu":               types.Int64Type,
		"memory":            types.Int64Type,
		"external_ip":       types.StringType,
		"created_at":        types.StringType,
	}
	_ = itemAttrs // referenced below via NestedAttributeObject

	resp.Schema = schema.Schema{
		Description: "List all vFirewall instances in a ViettelIDC VPC (read-only).",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID to filter firewalls. Falls back to provider default.",
			},
			"items": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of vFirewall instances.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                schema.StringAttribute{Computed: true, Description: "vFirewall ID."},
						"name":              schema.StringAttribute{Computed: true, Description: "vFirewall name."},
						"status":            schema.StringAttribute{Computed: true, Description: "Power state (POWERED_ON, POWERED_OFF, ...)."},
						"availability_zone": schema.StringAttribute{Computed: true, Description: "Availability zone."},
						"cpu":               schema.Int64Attribute{Computed: true, Description: "Number of vCPUs."},
						"memory":            schema.Int64Attribute{Computed: true, Description: "Memory in GB."},
						"external_ip":       schema.StringAttribute{Computed: true, Description: "External (management) IP address of the firewall."},
						"created_at":        schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
					},
				},
			},
		},
	}
}

func (d *VFirewallsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *VFirewallsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VFirewallsDataSourceModel
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
		"pageIndex":   0,
		"pageSize":    1000,
		"filters":     []interface{}{},
	}
	apiResp, diags := callAPI(ctx, d.client, pathVFirewallList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeVFirewallList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode vfirewall list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.Items = items
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ---------- Helpers ----------

func decodeVFirewallList(resp *client.APIResponse) ([]VFirewallItem, error) {
	// Response is { "pageIndex":0, "pageSize":10, "totalItems":N, "items":[...] }
	var wrapper struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &wrapper); err != nil {
		return nil, fmt.Errorf("decode vfirewall list: %w", err)
	}

	result := make([]VFirewallItem, 0, len(wrapper.Items))
	for _, raw := range wrapper.Items {
		var data map[string]interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			continue
		}

		// Extract external IP from networks[0].ipAddress
		externalIP := ""
		if nets, ok := data["networks"].([]interface{}); ok && len(nets) > 0 {
			if net, ok := nets[0].(map[string]interface{}); ok {
				externalIP = asString(net, "ipAddress")
			}
		}

		result = append(result, VFirewallItem{
			ID:               types.StringValue(asIDString(data, "id")),
			Name:             types.StringValue(asString(data, "name")),
			Status:           types.StringValue(asString(data, "status")),
			AvailabilityZone: types.StringValue(asString(data, "availabilityZone")),
			CPU:              types.Int64Value(asInt64(data, "cpu")),
			Memory:           types.Int64Value(asInt64(data, "memory")),
			ExternalIP:       types.StringValue(externalIP),
			CreatedAt:        types.StringValue(asString(data, "createdAt")),
		})
	}
	return result, nil
}
