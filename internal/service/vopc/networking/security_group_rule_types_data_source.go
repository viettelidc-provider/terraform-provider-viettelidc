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
	_ datasource.DataSource              = (*SGRuleTypesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*SGRuleTypesDataSource)(nil)
)

// SGRuleTypesDataSource implements `data "viettelidc_ovpc_security_group_rule_types"`.
// It queries /csa/api/v1/networking/security-group/rule/type which lists all
// available rule types that can be used as rule_type in viettelidc_ovpc_security_group_rule.
type SGRuleTypesDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// SGRuleTypesDataSourceModel is the top-level schema model.
type SGRuleTypesDataSourceModel struct {
	VpcID     types.String     `tfsdk:"vpc_id"`
	RuleTypes []SGRuleTypeItem `tfsdk:"rule_types"`
}

// SGRuleTypeItem represents one rule type returned by the API.
type SGRuleTypeItem struct {
	ID              types.Int64  `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	DefaultPort     types.String `tfsdk:"default_port"`
	DefaultProtocol types.Int64  `tfsdk:"default_protocol"`
	PortEnabled     types.Bool   `tfsdk:"port_enabled"`
	ProtocolEnabled types.Bool   `tfsdk:"protocol_enabled"`
}

// NewSGRuleTypesDataSource constructs the data source.
func NewSGRuleTypesDataSource() datasource.DataSource { return &SGRuleTypesDataSource{} }

func (d *SGRuleTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_security_group_rule_types"
}

func (d *SGRuleTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all available Security Group rule types. Use the `name` of each item as the `rule_type` argument in `viettelidc_ovpc_security_group_rule`.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID; falls back to the provider default vpc_id.",
			},
			"rule_types": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of all available rule types.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:    true,
							Description: "Internal ID of the rule type.",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Rule type name — use this as rule_type in viettelidc_ovpc_security_group_rule.",
						},
						"default_port": schema.StringAttribute{
							Computed:    true,
							Description: "Default port for this rule type (e.g. \"22\", \"80\", \"Any\").",
						},
						"default_protocol": schema.Int64Attribute{
							Computed:    true,
							Description: "Internal protocol ID.",
						},
						"port_enabled": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether a custom port can be specified for this rule type.",
						},
						"protocol_enabled": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether a custom protocol can be specified for this rule type.",
						},
					},
				},
			},
		},
	}
}

func (d *SGRuleTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *SGRuleTypesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg SGRuleTypesDataSourceModel
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
		"vpcId":      vpcID,
		"customerId": d.customerID,
	}

	// The rule/type endpoint returns a plain JSON array, not a wrapped APIResponse.
	raw, err := d.client.Do(ctx, pathSGRuleTypes, body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list Security Group rule types", err.Error())
		return
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(raw, &items); err != nil {
		resp.Diagnostics.AddError("Failed to parse Security Group rule types response", fmt.Sprintf("%s\nraw: %s", err, string(raw)))
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.RuleTypes = make([]SGRuleTypeItem, 0, len(items))
	for _, item := range items {
		cfg.RuleTypes = append(cfg.RuleTypes, SGRuleTypeItem{
			ID:              types.Int64Value(asInt64(item, "id")),
			Name:            types.StringValue(asString(item, "name")),
			DefaultPort:     types.StringValue(asString(item, "defaultPort")),
			DefaultProtocol: types.Int64Value(asInt64(item, "defaultProtocol")),
			PortEnabled:     types.BoolValue(asBool(item, "portEnabled")),
			ProtocolEnabled: types.BoolValue(asBool(item, "protocolEnabled")),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
