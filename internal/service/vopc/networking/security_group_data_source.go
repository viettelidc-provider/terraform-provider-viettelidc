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
	_ datasource.DataSource              = (*SecurityGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*SecurityGroupDataSource)(nil)
)

// SecurityGroupDataSource implements `data "viettelidc_security_group"` (lookup by id or name).
type SecurityGroupDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type SecurityGroupDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	VpcID       types.String `tfsdk:"vpc_id"`
}

func NewSecurityGroupDataSource() datasource.DataSource { return &SecurityGroupDataSource{} }

func (d *SecurityGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_security_group"
}

func (d *SecurityGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ViettelIDC Security Group by id or name.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Security Group ID (vttSecurityGroupId). Mutually optional with name."},
			"name":        schema.StringAttribute{Optional: true, Computed: true, Description: "Security Group name. Mutually optional with id."},
			"description": schema.StringAttribute{Computed: true},
			"vpc_id":      schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

func (d *SecurityGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *SecurityGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg SecurityGroupDataSourceModel
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
			"security_group_id": cfg.ID.ValueString(),
			"vpc_id":            vpcID,
			"customer_id":       d.customerID,
		}
		apiResp, diags := callAPI(ctx, d.client, pathSGDetail, body)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		m := &SecurityGroupResourceModel{}
		if err := mapSGResponse(apiResp, m); err != nil {
			resp.Diagnostics.AddError("decode security group detail", err.Error())
			return
		}
		cfg.ID = m.ID
		cfg.Name = m.Name
		cfg.Description = m.Description
		cfg.VpcID = types.StringValue(vpcID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
		return
	}

	// Lookup by name: list + client-side filter.
	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathSGList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeSGList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode security group list", err.Error())
		return
	}

	targetName := cfg.Name.ValueString()
	for _, sg := range items {
		if asString(sg, "name") == targetName {
			m := &SecurityGroupResourceModel{}
			if idStr := asIDString(sg, "vttSecurityGroupId"); idStr != "" {
				m.ID = types.StringValue(idStr)
			}
			m.Name = types.StringValue(asString(sg, "name"))
			m.Description = types.StringValue(asString(sg, "description"))
			cfg.ID = m.ID
			cfg.Name = m.Name
			cfg.Description = m.Description
			cfg.VpcID = types.StringValue(vpcID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Security Group Not Found",
		fmt.Sprintf("security_group with name %q not found in VPC %s.", targetName, vpcID),
	)
}

func decodeSGList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	// CSA list response may be wrapped in data.content, items, data, or directly as an array.
	var raw interface{}
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	// Try data as array directly.
	if arr, ok := raw.([]interface{}); ok {
		return toMapSlice(arr), nil
	}
	// Try nested shapes: items, content, data.
	if m, ok := raw.(map[string]interface{}); ok {
		for _, key := range []string{"items", "content", "data"} {
			if v, ok := m[key].([]interface{}); ok {
				return toMapSlice(v), nil
			}
		}
	}
	return nil, fmt.Errorf("unexpected list structure: %T", raw)
}

func toMapSlice(arr []interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}
