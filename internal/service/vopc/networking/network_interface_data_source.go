// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*NetworkInterfaceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*NetworkInterfaceDataSource)(nil)
)

// NetworkInterfaceDataSource implements `data "viettelidc_network_interface"` (lookup by id only).
type NetworkInterfaceDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type NetworkInterfaceDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	SubnetID     types.String `tfsdk:"subnet_id"`
	IpAssignType types.String `tfsdk:"ip_assign_type"`
	IpAddress    types.String `tfsdk:"ip_address"`
	VpcID        types.String `tfsdk:"vpc_id"`
	Description  types.String `tfsdk:"description"`
	Status       types.String `tfsdk:"status"`
}

func NewNetworkInterfaceDataSource() datasource.DataSource { return &NetworkInterfaceDataSource{} }

func (d *NetworkInterfaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_network_interface"
}

func (d *NetworkInterfaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a single ViettelIDC NIC by id.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Required: true, Description: "NIC ID (vttNetworkInterfaceId)."},
			"name":           schema.StringAttribute{Computed: true},
			"subnet_id":      schema.StringAttribute{Computed: true},
			"ip_assign_type": schema.StringAttribute{Computed: true},
			"ip_address":     schema.StringAttribute{Computed: true},
			"vpc_id":         schema.StringAttribute{Optional: true, Computed: true},
			"description":    schema.StringAttribute{Computed: true},
			"status":         schema.StringAttribute{Computed: true},
		},
	}
}

func (d *NetworkInterfaceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *NetworkInterfaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg NetworkInterfaceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing required attribute", "'id' is required.")
		return
	}
	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]interface{}{
		"network_interface_id": cfg.ID.ValueString(),
		"vpc_id":               vpcID,
		"customer_id":          d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathNicDetail, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := mapNicDataSource(apiResp, &cfg); err != nil {
		resp.Diagnostics.AddError("decode NIC detail", err.Error())
		return
	}
	cfg.VpcID = types.StringValue(vpcID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

func mapNicDataSource(resp *client.APIResponse, m *NetworkInterfaceDataSourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	fillNicDSFromMap(data, m)
	return nil
}

func fillNicDSFromMap(data map[string]interface{}, m *NetworkInterfaceDataSourceModel) {
	// Real API detail: "id" is the NIC ID (string). Fake-api uses "vttNetworkInterfaceId".
	if id, ok := data["id"].(string); ok && id != "" {
		m.ID = types.StringValue(id)
	} else {
		m.ID = types.StringValue(asIDString(data, "vttNetworkInterfaceId"))
	}
	m.Name = types.StringValue(asString(data, "name"))
	// Real API returns vttSubnetId as an integer; asIDString handles both int and string.
	m.SubnetID = types.StringValue(asIDString(data, "vttSubnetId"))
	m.IpAssignType = types.StringValue(asString(data, "ipAssignType"))
	m.IpAddress = types.StringValue(asString(data, "ipAddress"))
	// vpcId in NIC detail is 0 (not the real VPC); only update if non-zero.
	if v := asIDString(data, "vpcId"); v != "" {
		m.VpcID = types.StringValue(v)
	}
	m.Description = types.StringValue(asString(data, "description"))
	m.Status = types.StringValue(asString(data, "status"))
}

// decodeNicList accepts both array and object-with-array API response shapes.
func decodeNicList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	if len(resp.Data) == 0 {
		return nil, nil
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &arr); err == nil {
		return arr, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &obj); err != nil {
		return nil, err
	}
	for _, key := range []string{"networkInterfaces", "items", "list", "nics"} {
		if raw, ok := obj[key]; ok {
			var inner []map[string]interface{}
			if err := json.Unmarshal(raw, &inner); err == nil {
				return inner, nil
			}
		}
	}
	return nil, fmt.Errorf("unrecognised list shape: %s", string(resp.Data))
}
