package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ datasource.DataSource              = (*VMTemplatesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VMTemplatesDataSource)(nil)
)

// VMTemplatesDataSource implements `data "viettelidc_vm_templates"`.
// It queries /csa/api/v1/host-information/list-template which lists available
// OS/flavor templates that can be used as template_id in viettelidc_instance.
type VMTemplatesDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// VMTemplatesDataSourceModel is the schema model for this data source.
type VMTemplatesDataSourceModel struct {
	// Optional filters
	NameFilter types.String `tfsdk:"name_filter"`
	PageSize   types.Int64  `tfsdk:"page_size"`
	HostID     types.Int64  `tfsdk:"host_id"`
	VpcID      types.String `tfsdk:"vpc_id"`
	// Computed outputs
	Templates []VMTemplateItem `tfsdk:"templates"`
}

// VMTemplateItem represents one OS/flavor template returned by the API.
type VMTemplateItem struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	OsType      types.String `tfsdk:"os_type"`
	CPU         types.Int64  `tfsdk:"cpu"`
	Memory      types.Int64  `tfsdk:"memory"`
}

// NewVMTemplatesDataSource constructs the data source.
func NewVMTemplatesDataSource() datasource.DataSource { return &VMTemplatesDataSource{} }

func (d *VMTemplatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_vm_templates"
}

func (d *VMTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List available VM OS/flavor templates. The id of each template is used as template_id in viettelidc_instance.",
		Attributes: map[string]schema.Attribute{
			"name_filter": schema.StringAttribute{
				Optional:    true,
				Description: "Filter templates by name (partial match, e.g. \"ubun\" for Ubuntu).",
			},
			"page_size": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum number of results to return (default 100).",
			},
			"host_id": schema.Int64Attribute{
				Optional:    true,
				Description: "Host ID (hypervisor cluster). Required by the API to return results (e.g. 6).",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID; falls back to the provider default vpc_id.",
			},
			"templates": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of matching VM templates.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, Description: "Template ID — use this as template_id in viettelidc_instance."},
						"name":        schema.StringAttribute{Computed: true, Description: "Template name (e.g. Ubuntu 22.04)."},
						"description": schema.StringAttribute{Computed: true, Description: "Template description."},
						"os_type":     schema.StringAttribute{Computed: true, Description: "OS type (e.g. Linux, Windows)."},
						"cpu":         schema.Int64Attribute{Computed: true, Description: "Number of vCPUs."},
						"memory":      schema.Int64Attribute{Computed: true, Description: "Memory in MB."},
					},
				},
			},
		},
	}
}

func (d *VMTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *VMTemplatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VMTemplatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	pageSize := int64(100)
	if !cfg.PageSize.IsNull() && !cfg.PageSize.IsUnknown() && cfg.PageSize.ValueInt64() > 0 {
		pageSize = cfg.PageSize.ValueInt64()
	}

	body := map[string]interface{}{
		"pageIndex":   0,
		"pageSize":    pageSize,
		"totalItems":  0,
		"filters":     []interface{}{},
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
	}
	if !cfg.HostID.IsNull() && !cfg.HostID.IsUnknown() {
		body["host_id"] = cfg.HostID.ValueInt64()
	}

	// Append name filter if provided.
	if !cfg.NameFilter.IsNull() && !cfg.NameFilter.IsUnknown() && cfg.NameFilter.ValueString() != "" {
		body["filters"] = []map[string]interface{}{
			{
				"name":   "name",
				"values": []string{cfg.NameFilter.ValueString()},
			},
		}
	}

	apiResp, diags := callAPI(ctx, d.client, pathVMTemplateList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeVMTemplateList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode vm template list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.Templates = make([]VMTemplateItem, 0, len(items))
	for _, raw := range items {
		id := asIDString(raw, "id")
		if id == "" {
			id = asIDString(raw, "templateId")
		}
		cfg.Templates = append(cfg.Templates, VMTemplateItem{
			ID:          types.StringValue(id),
			Name:        types.StringValue(asString(raw, "name")),
			Description: types.StringValue(asString(raw, "description")),
			OsType:      types.StringValue(asString(raw, "osType")),
			CPU:         types.Int64Value(asInt64(raw, "cpu")),
			Memory:      types.Int64Value(asInt64(raw, "memory")),
		})
	}

	if len(cfg.Templates) >= listWarningThreshold {
		resp.Diagnostics.AddWarning(
			"VM template list may be truncated",
			"Returned 1000 or more templates. Consider using name_filter or increasing page_size.",
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// decodeVMTemplateList handles both array and paged-object API response shapes.
func decodeVMTemplateList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}
	// Try bare array first.
	var arr []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &arr); err == nil {
		return arr, nil
	}
	// Paged envelope: {"items":[...], "totalItems":N} or {"list":[...]}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &obj); err != nil {
		return nil, fmt.Errorf("unexpected vm template list response shape: %s", string(resp.Data))
	}
	for _, key := range []string{"items", "list", "templates", "data"} {
		if raw, ok := obj[key]; ok {
			var inner []map[string]interface{}
			if err := json.Unmarshal(raw, &inner); err == nil {
				return inner, nil
			}
		}
	}
	return nil, fmt.Errorf("unrecognised vm template list shape: %s", string(resp.Data))
}
