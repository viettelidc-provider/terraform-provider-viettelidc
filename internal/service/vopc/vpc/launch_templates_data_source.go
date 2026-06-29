// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vpc

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// Compile-time interface assertions.
var (
	_ datasource.DataSource              = (*LaunchTemplatesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*LaunchTemplatesDataSource)(nil)
)

// LaunchTemplatesDataSource implements `data "viettelidc_launch_templates"` (list all).
type LaunchTemplatesDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// LaunchTemplateItem is one element in the list data source result.
type LaunchTemplateItem struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	VmID        types.String `tfsdk:"vm_id"`
	MemorySize  types.Int64  `tfsdk:"memory_size"`
	CpuSize     types.Int64  `tfsdk:"cpu_size"`
}

// LaunchTemplatesDataSourceModel is the schema model for the list data source.
type LaunchTemplatesDataSourceModel struct {
	VpcID           types.String         `tfsdk:"vpc_id"`
	LaunchTemplates []LaunchTemplateItem `tfsdk:"launch_templates"`
}

// NewLaunchTemplatesDataSource constructs the plural data source.
func NewLaunchTemplatesDataSource() datasource.DataSource { return &LaunchTemplatesDataSource{} }

func (d *LaunchTemplatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_launch_templates"
}

func (d *LaunchTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	itemAttrs := map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, Description: "Launch Template ID."},
		"name":        schema.StringAttribute{Computed: true, Description: "Template name."},
		"description": schema.StringAttribute{Computed: true, Description: "Description."},
		"vm_id":       schema.StringAttribute{Computed: true, Description: "Source VM ID."},
		"memory_size": schema.Int64Attribute{Computed: true, Description: "Memory size in GB."},
		"cpu_size":    schema.Int64Attribute{Computed: true, Description: "Number of vCPUs."},
	}
	resp.Schema = schema.Schema{
		Description: "List all ViettelIDC Launch Templates in a VPC.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{Optional: true, Computed: true, Description: "VPC filter; falls back to provider default."},
			"launch_templates": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: itemAttrs,
				},
			},
		},
	}
}

func (d *LaunchTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *LaunchTemplatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg LaunchTemplatesDataSourceModel
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

	cfg.VpcID = types.StringValue(vpcID)
	cfg.LaunchTemplates = make([]LaunchTemplateItem, 0, len(items))
	for _, raw := range items {
		item := LaunchTemplateItem{
			ID:          types.StringValue(asIDString(raw, "id")),
			Name:        types.StringValue(asString(raw, "name")),
			Description: types.StringValue(asString(raw, "description")),
			VmID:        types.StringValue(asString(raw, "vmId")),
			MemorySize:  types.Int64Value(asInt64(raw, "memorySize")),
			CpuSize:     types.Int64Value(asInt64(raw, "cpuSize")),
		}
		cfg.LaunchTemplates = append(cfg.LaunchTemplates, item)
	}

	if len(cfg.LaunchTemplates) >= listWarningThreshold {
		resp.Diagnostics.AddWarning(
			"Launch template list may be truncated",
			"Returned 1000 or more launch templates; the API may have applied a default page size.",
		)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// decodeLaunchTemplateList decodes a API list-all response into a slice of maps.
func decodeLaunchTemplateList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}
	// API list-all may return an array or a wrapper object.
	var items []map[string]interface{}
	if err := resp.ExtractData(&items); err == nil {
		return items, nil
	}
	// Try wrapper object: {"items": [...]} or {"data": [...]}
	var wrapper map[string]interface{}
	if err := resp.ExtractData(&wrapper); err != nil {
		return nil, err
	}
	for _, key := range []string{"items", "data", "launchTemplates"} {
		if raw, ok := wrapper[key]; ok {
			if arr, ok := raw.([]interface{}); ok {
				result := make([]map[string]interface{}, 0, len(arr))
				for _, v := range arr {
					if m, ok := v.(map[string]interface{}); ok {
						result = append(result, m)
					}
				}
				return result, nil
			}
		}
	}
	return nil, nil
}
