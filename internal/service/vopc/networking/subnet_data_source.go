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
	_ datasource.DataSource              = (*SubnetDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*SubnetDataSource)(nil)
)

// SubnetDataSource implements `data "viettelidc_subnet"` (singular lookup).
type SubnetDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// SubnetDataSourceModel mirrors the data source schema.
type SubnetDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	NetworkAddress types.String `tfsdk:"network_address"`
	IsPublicZone   types.Bool   `tfsdk:"is_public_zone"`
	VpcID          types.String `tfsdk:"vpc_id"`
	Description    types.String `tfsdk:"description"`
}

func NewSubnetDataSource() datasource.DataSource { return &SubnetDataSource{} }

func (d *SubnetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_subnet"
}

func (d *SubnetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a single ViettelIDC subnet by id or name.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Optional: true, Computed: true, Description: "Subnet ID (vttSubnetId). Mutually optional with name."},
			"name":            schema.StringAttribute{Optional: true, Computed: true, Description: "Subnet name. Mutually optional with id."},
			"network_address": schema.StringAttribute{Computed: true},
			"is_public_zone":  schema.BoolAttribute{Computed: true},
			"vpc_id":          schema.StringAttribute{Optional: true, Computed: true},
			"description":     schema.StringAttribute{Computed: true},
		},
	}
}

func (d *SubnetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *SubnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg SubnetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !cfg.ID.IsNull() && cfg.ID.ValueString() != ""
	hasName := !cfg.Name.IsNull() && cfg.Name.ValueString() != ""
	if !hasID && !hasName {
		resp.Diagnostics.AddError("Missing lookup key", "Set either 'id' or 'name'.")
		return
	}

	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if hasID {
		body := map[string]interface{}{
			"subnet_id":   cfg.ID.ValueString(),
			"vpc_id":      vpcID,
			"customer_id": d.customerID,
		}
		apiResp, diags := callAPI(ctx, d.client, pathSubnetDetail, body)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := mapSubnetDataSource(apiResp, &cfg); err != nil {
			resp.Diagnostics.AddError("decode subnet detail", err.Error())
			return
		}
		cfg.VpcID = types.StringValue(vpcID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
		return
	}

	// Lookup by name: list + client-side filter.
	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathSubnetList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := decodeSubnetList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode subnet list", err.Error())
		return
	}
	wanted := cfg.Name.ValueString()
	for _, item := range items {
		if asString(item, "name") == wanted {
			fillSubnetDSFromMap(item, &cfg)
			cfg.VpcID = types.StringValue(vpcID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
			return
		}
	}
	resp.Diagnostics.AddError("Subnet not found", fmt.Sprintf("no subnet with name=%q in vpc=%q", wanted, vpcID))
}

// ---- helpers shared with subnets list data source ----

func mapSubnetDataSource(resp *client.APIResponse, m *SubnetDataSourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return err
	}
	fillSubnetDSFromMap(data, m)
	return nil
}

func fillSubnetDSFromMap(data map[string]interface{}, m *SubnetDataSourceModel) {
	// Real API: "id" is string; "vttSubnetId" is integer. asIDString handles both.
	if id, ok := data["id"].(string); ok && id != "" {
		m.ID = types.StringValue(id)
	} else {
		m.ID = types.StringValue(asIDString(data, "vttSubnetId"))
	}
	m.Name = types.StringValue(asString(data, "name"))
	m.NetworkAddress = types.StringValue(asString(data, "networkAddress"))
	// Real API uses "isPublic"; fake-api / spec used "isPublicZone".
	if _, ok := data["isPublic"]; ok {
		m.IsPublicZone = types.BoolValue(asBool(data, "isPublic"))
	} else {
		m.IsPublicZone = types.BoolValue(asBool(data, "isPublicZone"))
	}
	if v := asIDString(data, "vpcId"); v != "" {
		m.VpcID = types.StringValue(v)
	}
	m.Description = types.StringValue(asString(data, "description"))
}

// decodeSubnetList accepts both array and object-with-array API response shapes.
func decodeSubnetList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	if len(resp.Data) == 0 {
		return nil, nil
	}
	// Try array first.
	var arr []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &arr); err == nil {
		return arr, nil
	}
	// Object with "subnets" array (or "items").
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &obj); err != nil {
		return nil, err
	}
	for _, key := range []string{"subnets", "items", "list"} {
		if raw, ok := obj[key]; ok {
			var inner []map[string]interface{}
			if err := json.Unmarshal(raw, &inner); err == nil {
				return inner, nil
			}
		}
	}
	return nil, fmt.Errorf("unrecognised list shape: %s", string(resp.Data))
}
