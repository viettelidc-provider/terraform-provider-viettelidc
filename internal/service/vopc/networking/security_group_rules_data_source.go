// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*SecurityGroupRulesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*SecurityGroupRulesDataSource)(nil)
)

// SecurityGroupRulesDataSource implements `data "viettelidc_ovpc_security_group_rules"`.
// One data source covers both directions; direction picks the endpoint.
type SecurityGroupRulesDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type SecurityGroupRulesDataSourceModel struct {
	SecurityGroupID types.String            `tfsdk:"security_group_id"`
	Direction       types.String            `tfsdk:"direction"`
	VpcID           types.String            `tfsdk:"vpc_id"`
	Rules           []SecurityGroupRuleItem `tfsdk:"rules"`
}

type SecurityGroupRuleItem struct {
	ID            types.String `tfsdk:"id"`
	Direction     types.String `tfsdk:"direction"`
	RuleType      types.String `tfsdk:"rule_type"`
	EtherType     types.String `tfsdk:"ether_type"`
	ProtocolName  types.String `tfsdk:"protocol_name"`
	Port          types.String `tfsdk:"port"`
	SourceIP      types.String `tfsdk:"source_ip"`
	DestinationIP types.String `tfsdk:"destination_ip"`
	Status        types.String `tfsdk:"status"`
}

func NewSecurityGroupRulesDataSource() datasource.DataSource {
	return &SecurityGroupRulesDataSource{}
}

func (d *SecurityGroupRulesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_security_group_rules"
}

func (d *SecurityGroupRulesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the inbound or outbound rules of a ViettelIDC Security Group.",
		Attributes: map[string]schema.Attribute{
			"security_group_id": schema.StringAttribute{
				Required:    true,
				Description: "Security Group whose rules to list.",
			},
			"direction": schema.StringAttribute{
				Required:    true,
				Description: `Which rules to return: "in" (inbound) or "out" (outbound).`,
				Validators: []validator.String{
					stringvalidator.OneOf("in", "out", "inbound", "outbound", "ingress", "egress"),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Description: "VPC ID; falls back to the provider default vpc_id.",
			},
			"rules": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true},
						"direction":      schema.StringAttribute{Computed: true},
						"rule_type":      schema.StringAttribute{Computed: true},
						"ether_type":     schema.StringAttribute{Computed: true},
						"protocol_name":  schema.StringAttribute{Computed: true},
						"port":           schema.StringAttribute{Computed: true},
						"source_ip":      schema.StringAttribute{Computed: true},
						"destination_ip": schema.StringAttribute{Computed: true},
						"status":         schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *SecurityGroupRulesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *SecurityGroupRulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg SecurityGroupRulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	dir := normalizeDirection(cfg.Direction.ValueString())
	path := pathSGRuleInboundList
	if dir == "out" {
		path = pathSGRuleOutboundList
	}

	body := map[string]interface{}{
		"security_group_id": cfg.SecurityGroupID.ValueString(),
		"vpc_id":            vpcID,
		"customer_id":       d.customerID,
		"pageIndex":         0,
		"pageSize":          1000,
	}
	apiResp, diags := callAPI(ctx, d.client, path, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// decodeSubnetList is shape-generic: array, or object with items/list.
	items, err := decodeSubnetList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode security group rule list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.Direction = types.StringValue(dir)
	cfg.Rules = make([]SecurityGroupRuleItem, 0, len(items))
	for _, raw := range items {
		cfg.Rules = append(cfg.Rules, SecurityGroupRuleItem{
			ID:            types.StringValue(asIDString(raw, "vttSecurityRuleId")),
			Direction:     types.StringValue(strings.ToLower(asString(raw, "direction"))),
			RuleType:      types.StringValue(asString(raw, "type")),
			EtherType:     types.StringValue(asString(raw, "etherType")),
			ProtocolName:  types.StringValue(asString(raw, "protocolName")),
			Port:          types.StringValue(asString(raw, "port")),
			SourceIP:      types.StringValue(asString(raw, "sourceIP")),
			DestinationIP: types.StringValue(asString(raw, "destinationIP")),
			Status:        types.StringValue(asString(raw, "status")),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
