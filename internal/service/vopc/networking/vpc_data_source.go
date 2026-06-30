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
	_ datasource.DataSource              = (*VPCDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VPCDataSource)(nil)
)

// VPCDataSource implements `data "viettelidc_vpc"` — read-only lookup of an
// existing VPC. Use this when VPC is pre-provisioned outside Terraform.
type VPCDataSource struct {
	client     *client.Client
	customerID string
}

type VPCDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	CidrBlock   types.String `tfsdk:"cidr_block"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
}

func NewVPCDataSource() datasource.DataSource { return &VPCDataSource{} }

func (d *VPCDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_vpc"
}

func (d *VPCDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing ViettelIDC VPC by id or name.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Optional: true, Computed: true, Description: "VPC ID. Mutually optional with name."},
			"name":        schema.StringAttribute{Optional: true, Computed: true, Description: "VPC name. Mutually optional with id."},
			"cidr_block":  schema.StringAttribute{Computed: true, Description: "CIDR block of the VPC."},
			"description": schema.StringAttribute{Computed: true, Description: "VPC description."},
			"status":      schema.StringAttribute{Computed: true, Description: "VPC status."},
		},
	}
}

func (d *VPCDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
}

func (d *VPCDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VPCDataSourceModel
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

	if hasID {
		body := map[string]interface{}{
			"id":          cfg.ID.ValueString(),
			"vpc_id":      cfg.ID.ValueString(),
			"customer_id": d.customerID,
		}
		apiResp, diags := callAPI(ctx, d.client, pathVPCDetail, body)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := mapVPCDataSource(apiResp, &cfg); err != nil {
			resp.Diagnostics.AddError("decode vpc detail", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
		return
	}

	// Lookup by name: list all VPCs then filter client-side.
	body := map[string]interface{}{
		"customer_id": d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathVPCList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeVPCList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode vpc list", err.Error())
		return
	}

	var found *VPCDataSourceModel
	for i := range items {
		if items[i].Name.ValueString() == cfg.Name.ValueString() {
			found = &items[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("VPC not found", fmt.Sprintf("No VPC named %q found.", cfg.Name.ValueString()))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, found)...)
}

// ---------- Helpers ----------

func mapVPCDataSource(resp *client.APIResponse, m *VPCDataSourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	} else if id := asIDString(data, "vttVpcId"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	if v := asString(data, "cidrBlock"); v != "" {
		m.CidrBlock = types.StringValue(v)
	} else if v := asString(data, "cidr_block"); v != "" {
		m.CidrBlock = types.StringValue(v)
	}
	m.Description = types.StringValue(asString(data, "description"))
	m.Status = types.StringValue(asString(data, "status"))
	return nil
}

func decodeVPCList(resp *client.APIResponse) ([]VPCDataSourceModel, error) {
	// Try top-level array first, then wrapped {"items":[...]} or {"data":[...]}
	var raw []json.RawMessage
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		var wrapper map[string]json.RawMessage
		if err2 := json.Unmarshal(resp.Data, &wrapper); err2 != nil {
			return nil, fmt.Errorf("decode vpc list: %w", err2)
		}
		for _, key := range []string{"items", "data", "vpcs"} {
			if v, ok := wrapper[key]; ok {
				if err3 := json.Unmarshal(v, &raw); err3 != nil {
					return nil, fmt.Errorf("decode vpc list[%s]: %w", key, err3)
				}
				break
			}
		}
	}

	result := make([]VPCDataSourceModel, 0, len(raw))
	for _, item := range raw {
		var data map[string]interface{}
		if err := json.Unmarshal(item, &data); err != nil {
			continue
		}
		var m VPCDataSourceModel
		if id := asIDString(data, "id"); id != "" {
			m.ID = types.StringValue(id)
		} else if id := asIDString(data, "vttVpcId"); id != "" {
			m.ID = types.StringValue(id)
		}
		m.Name = types.StringValue(asString(data, "name"))
		if v := asString(data, "cidrBlock"); v != "" {
			m.CidrBlock = types.StringValue(v)
		} else {
			m.CidrBlock = types.StringValue(asString(data, "cidr_block"))
		}
		m.Description = types.StringValue(asString(data, "description"))
		m.Status = types.StringValue(asString(data, "status"))
		result = append(result, m)
	}
	return result, nil
}
