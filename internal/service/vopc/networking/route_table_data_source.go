// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*RouteTableDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*RouteTableDataSource)(nil)
)

// RouteTableDataSource implements `data "viettelidc_route_table"`.
// Lookup by id (detail) or by name (list + client-side filter).
type RouteTableDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type RouteTableDataSourceModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	VpcID types.String `tfsdk:"vpc_id"`
}

func NewRouteTableDataSource() datasource.DataSource { return &RouteTableDataSource{} }

func (d *RouteTableDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_route_table"
}

func (d *RouteTableDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ViettelIDC Route Table by id or name.",
		Attributes: map[string]schema.Attribute{
			"id":     schema.StringAttribute{Optional: true, Computed: true, Description: "Route Table ID. Mutually optional with name."},
			"name":   schema.StringAttribute{Optional: true, Computed: true, Description: "Route Table name. Mutually optional with id."},
			"vpc_id": schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

func (d *RouteTableDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *RouteTableDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg RouteTableDataSourceModel
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
			"route_table_id": cfg.ID.ValueString(),
			"vpc_id":         vpcID,
			"customer_id":    d.customerID,
		}
		apiResp, diags := callAPI(ctx, d.client, pathRouteTableDetail, body)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		m := &RouteTableResourceModel{}
		if err := mapRouteTableResponse(apiResp, m); err != nil {
			resp.Diagnostics.AddError("decode route table detail", err.Error())
			return
		}
		cfg.ID = m.ID
		cfg.Name = m.Name
		cfg.VpcID = types.StringValue(vpcID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
		return
	}

	// Lookup by name via list.
	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathRouteTableList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeSGList(apiResp) // reuse generic list decoder
	if err != nil {
		resp.Diagnostics.AddError("decode route table list", err.Error())
		return
	}

	targetName := cfg.Name.ValueString()
	for _, item := range items {
		if asString(item, "name") == targetName {
			cfg.ID = types.StringValue(asIDString(item, "vttRouteTableId"))
			if cfg.ID.ValueString() == "" {
				cfg.ID = types.StringValue(asIDString(item, "id"))
			}
			cfg.Name = types.StringValue(asString(item, "name"))
			cfg.VpcID = types.StringValue(vpcID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Route Table Not Found",
		fmt.Sprintf("route_table with name %q not found in VPC %s.", targetName, vpcID),
	)
}
