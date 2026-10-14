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

// NetworkInterfaceDataSource implements `data "viettelidc_ovpc_network_interface"` (lookup by id, name, or ip_address).
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
		Description: "Look up a single ViettelIDC NIC by id, name, or ip_address.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Optional: true, Computed: true, Description: "NIC ID (vttNetworkInterfaceId). At least one of 'id', 'name', or 'ip_address' must be specified."},
			"name":           schema.StringAttribute{Optional: true, Computed: true, Description: "NIC name. At least one of 'id', 'name', or 'ip_address' must be specified."},
			"subnet_id":      schema.StringAttribute{Computed: true},
			"ip_assign_type": schema.StringAttribute{Computed: true},
			"ip_address":     schema.StringAttribute{Optional: true, Computed: true, Description: "Private IPv4 address of the NIC. At least one of 'id', 'name', or 'ip_address' must be specified."},
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
	hasID := !cfg.ID.IsNull() && !cfg.ID.IsUnknown() && cfg.ID.ValueString() != ""
	hasName := !cfg.Name.IsNull() && !cfg.Name.IsUnknown() && cfg.Name.ValueString() != ""
	hasIP := !cfg.IpAddress.IsNull() && !cfg.IpAddress.IsUnknown() && cfg.IpAddress.ValueString() != ""
	if !hasID && !hasName && !hasIP {
		resp.Diagnostics.AddError("Missing lookup key", "Set at least one of 'id', 'name', or 'ip_address'.")
		return
	}

	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if hasID {
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

	wantName := cfg.Name.ValueString()
	wantIP := cfg.IpAddress.ValueString()
	var found map[string]interface{}
	for _, raw := range items {
		if hasName && asString(raw, "name") != wantName {
			continue
		}
		if hasIP {
			itemIP := asString(raw, "ipAddress")
			if itemIP == "" || itemIP == "default" {
				itemIP = asString(raw, "primaryIp4")
			}
			if itemIP != wantIP {
				continue
			}
		}
		found = raw
		break
	}
	if found == nil {
		resp.Diagnostics.AddError(
			"NIC not found",
			fmt.Sprintf("No NIC matching name=%q ip_address=%q in VPC %s.", wantName, wantIP, vpcID),
		)
		return
	}

	fillNicDSFromMap(found, &cfg)
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
	if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	} else {
		m.ID = types.StringValue(asIDString(data, "vttNetworkInterfaceId"))
	}
	m.Name = types.StringValue(asString(data, "name"))
	// Real API returns vttSubnetId as an integer; asIDString handles both int and string.
	m.SubnetID = types.StringValue(asIDString(data, "vttSubnetId"))
	m.IpAssignType = types.StringValue(asString(data, "ipAssignType"))
	ip := asString(data, "ipAddress")
	if ip == "" || ip == "default" {
		ip = asString(data, "primaryIp4")
	}
	if ip == "default" {
		ip = ""
	}
	m.IpAddress = types.StringValue(ip)
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
