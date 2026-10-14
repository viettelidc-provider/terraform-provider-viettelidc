// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// Compile-time interface assertions.
var (
	_ datasource.DataSource              = (*LaunchTemplateDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*LaunchTemplateDataSource)(nil)
)

// LaunchTemplateDataSource implements `data "viettelidc_ovpc_launch_template"` (lookup by ID or name).
type LaunchTemplateDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// LaunchTemplateDataSourceModel is the schema model for the singular data source.
type LaunchTemplateDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	VmID        types.String `tfsdk:"vm_id"`
	MemorySize  types.Int64  `tfsdk:"memory_size"`
	CpuSize     types.Int64  `tfsdk:"cpu_size"`
	VpcID       types.String `tfsdk:"vpc_id"`
}

// NewLaunchTemplateDataSource constructs the singular data source.
func NewLaunchTemplateDataSource() datasource.DataSource { return &LaunchTemplateDataSource{} }

func (d *LaunchTemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_launch_template"
}

func (d *LaunchTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Read a ViettelIDC Launch Template by its ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Launch Template ID. Either 'id' or 'name' must be specified."},
			"name":        schema.StringAttribute{Optional: true, Computed: true, Description: "Template name. Either 'id' or 'name' must be specified."},
			"description": schema.StringAttribute{Computed: true, Description: "Description."},
			"vm_id":       schema.StringAttribute{Computed: true, Description: "Source VM ID."},
			"memory_size": schema.Int64Attribute{Computed: true, Description: "Memory size in GB."},
			"cpu_size":    schema.Int64Attribute{Computed: true, Description: "Number of vCPUs."},
			"vpc_id":      schema.StringAttribute{Optional: true, Computed: true, Description: "VPC ID."},
		},
	}
}

func (d *LaunchTemplateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *LaunchTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg LaunchTemplateDataSourceModel
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
			"id":          cfg.ID.ValueString(),
			"vpc_id":      vpcID,
			"customer_id": d.customerID,
		}
		apiResp, diags := callAPI(ctx, d.client, pathLaunchTemplateDetail, body)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		if err := mapLaunchTemplateToDataSource(apiResp, &cfg, vpcID); err != nil {
			resp.Diagnostics.AddError("Decode launch template detail", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
		return
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathLaunchTemplateListAll, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeLaunchTemplateList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Decode launch template list", err.Error())
		return
	}

	wantName := cfg.Name.ValueString()
	var matches []map[string]interface{}
	for _, raw := range items {
		if asString(raw, "name") == wantName {
			matches = append(matches, raw)
		}
	}
	if len(matches) == 0 {
		resp.Diagnostics.AddError("Launch Template not found", fmt.Sprintf("No launch template named %q found in VPC %s.", wantName, vpcID))
		return
	}
	if len(matches) > 1 {
		resp.Diagnostics.AddError("Ambiguous Launch Template name", fmt.Sprintf("Found %d launch templates named %q in VPC %s; specify 'id' instead.", len(matches), wantName, vpcID))
		return
	}

	fillLaunchTemplateMapToDataSource(matches[0], &cfg, vpcID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// mapLaunchTemplateToDataSource populates the data source model from a API detail response.
func mapLaunchTemplateToDataSource(resp *client.APIResponse, m *LaunchTemplateDataSourceModel, vpcID string) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	fillLaunchTemplateMapToDataSource(data, m, vpcID)
	return nil
}

func fillLaunchTemplateMapToDataSource(data map[string]interface{}, m *LaunchTemplateDataSourceModel, vpcID string) {
	if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	}
	m.Name = types.StringValue(asString(data, "name"))
	if v, ok := data["description"]; ok {
		if s, ok := v.(string); ok {
			m.Description = types.StringValue(s)
		}
	}
	if m.Description.IsNull() || m.Description.IsUnknown() {
		m.Description = types.StringValue("")
	}
	if v := asString(data, "vmId"); v != "" {
		m.VmID = types.StringValue(v)
	} else if m.VmID.IsNull() || m.VmID.IsUnknown() {
		m.VmID = types.StringValue("")
	}
	m.MemorySize = types.Int64Value(asInt64(data, "memorySize"))
	m.CpuSize = types.Int64Value(asInt64(data, "cpuSize"))
	if v := asString(data, "vpcId"); v != "" {
		m.VpcID = types.StringValue(v)
	} else {
		m.VpcID = types.StringValue(vpcID)
	}
}
