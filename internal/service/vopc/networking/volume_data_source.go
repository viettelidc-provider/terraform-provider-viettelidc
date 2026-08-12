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
	_ datasource.DataSource              = (*VolumeDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VolumeDataSource)(nil)
)

// VolumeDataSource implements `data "viettelidc_ovpc_volume"` (lookup by id or name).
type VolumeDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type VolumeDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Size       types.Int64  `tfsdk:"size"`
	VolumeType types.String `tfsdk:"volume_type"`
	Status     types.String `tfsdk:"status"`
	VpcID      types.String `tfsdk:"vpc_id"`
}

func NewVolumeDataSource() datasource.DataSource { return &VolumeDataSource{} }

func (d *VolumeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_volume"
}

func (d *VolumeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a single ViettelIDC block storage volume by id or name.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Volume ID (vttVolumeId). Provide id or name."},
			"name":        schema.StringAttribute{Optional: true, Computed: true, Description: "Volume name. Provide id or name."},
			"size":        schema.Int64Attribute{Computed: true, Description: "Volume size in GiB."},
			"volume_type": schema.StringAttribute{Computed: true},
			"status":      schema.StringAttribute{Computed: true},
			"vpc_id":      schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

func (d *VolumeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *VolumeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VolumeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := cfg.ID.ValueString()
	name := cfg.Name.ValueString()
	if id == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either 'id' or 'name'.")
		return
	}
	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var found map[string]interface{}
	if id != "" {
		// Lookup by id via volume/detail.
		apiResp, dd := callAPI(ctx, d.client, pathVolumeDetail, map[string]interface{}{
			"volume_id":   id,
			"vpc_id":      vpcID,
			"customer_id": d.customerID,
		})
		resp.Diagnostics.Append(dd...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := json.Unmarshal(apiResp.Data, &found); err != nil {
			resp.Diagnostics.AddError("decode volume detail", err.Error())
			return
		}
	} else {
		// Lookup by name via volume/list. The API "name" filter is a substring
		// match, so it only narrows the page — we still match exactly below.
		apiResp, dd := callAPI(ctx, d.client, pathVolumeList, map[string]interface{}{
			"vpc_id":      vpcID,
			"customer_id": d.customerID,
			"pageIndex":   0,
			"pageSize":    1000,
			"filters":     []map[string]interface{}{{"name": "name", "values": []string{name}}},
		})
		resp.Diagnostics.Append(dd...)
		if resp.Diagnostics.HasError() {
			return
		}
		items, err := decodeSubnetList(apiResp) // shape-generic list decoder
		if err != nil {
			resp.Diagnostics.AddError("decode volume list", err.Error())
			return
		}
		for _, raw := range items {
			if asString(raw, "name") == name {
				found = raw
				break
			}
		}
		if found == nil {
			resp.Diagnostics.AddError("Volume not found", fmt.Sprintf("no volume named %q in vpc %s", name, vpcID))
			return
		}
	}

	fillVolumeDS(found, &cfg)
	cfg.VpcID = types.StringValue(vpcID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

func fillVolumeDS(data map[string]interface{}, m *VolumeDataSourceModel) {
	if id := asIDString(data, "vttVolumeId"); id != "" {
		m.ID = types.StringValue(id)
	} else if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	}
	m.Name = types.StringValue(asString(data, "name"))
	if v := asIDString(data, "size"); v != "" {
		if n, err := parseInt64(v); err == nil {
			m.Size = types.Int64Value(n)
		}
	} else if v, ok := data["size"].(float64); ok {
		m.Size = types.Int64Value(int64(v))
	}
	if v := asString(data, "volumeDisplayType"); v != "" {
		m.VolumeType = types.StringValue(v)
	} else {
		m.VolumeType = types.StringValue(asString(data, "volumeType"))
	}
	m.Status = types.StringValue(asString(data, "status"))
}
