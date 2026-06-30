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
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

type HostModel struct {
	ID   types.String `json:"id" tfsdk:"id"`
	Name types.String `json:"name" tfsdk:"name"`
	IP   types.String `json:"ip" tfsdk:"ip"`
}

// --- HOSTS BY ORDER ---

func NewHostsByOrderDataSource() datasource.DataSource {
	return &hostsByOrderDataSource{}
}

type hostsByOrderDataSource struct {
	clientData *providerdata.ProviderData
}

type HostsByOrderDSModel struct {
	ID         types.String `tfsdk:"id"`
	ProviderID types.Int64  `tfsdk:"provider_id"`
	CustomerID types.String `tfsdk:"customer_id"`
	VpcID      types.String `tfsdk:"vpc_id"`
	Hosts      []HostModel  `tfsdk:"hosts"`
}

func (d *hostsByOrderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_hosts_by_order"
}

func (d *hostsByOrderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *hostsByOrderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing hosts by order.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"provider_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Provider ID. Defaults to 7.",
			},
			"customer_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Customer ID. Defaults to provider config.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Defaults to provider config.",
			},
			"hosts": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"ip": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *hostsByOrderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config HostsByOrderDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := d.clientData.CustomerID
	if !config.CustomerID.IsNull() && !config.CustomerID.IsUnknown() && config.CustomerID.ValueString() != "" {
		customerID = config.CustomerID.ValueString()
	}

	vpcID := d.clientData.DefaultVpcID
	if !config.VpcID.IsNull() && !config.VpcID.IsUnknown() && config.VpcID.ValueString() != "" {
		vpcID = config.VpcID.ValueString()
	}

	providerID := int64(7)
	if !config.ProviderID.IsNull() && !config.ProviderID.IsUnknown() {
		providerID = config.ProviderID.ValueInt64()
	}

	payload := map[string]interface{}{
		"provider_id": providerID,
		"vpc_id":      vpcID,
		"customer_id": customerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathRegionHostsByOrder, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listData []interface{}
	if err := json.Unmarshal(apiResp.Data, &listData); err == nil {
		config.ID = types.StringValue(fmt.Sprintf("%d-%s-%s", providerID, vpcID, customerID))
		config.CustomerID = types.StringValue(customerID)
		config.ProviderID = types.Int64Value(providerID)
		config.VpcID = types.StringValue(vpcID)
		config.Hosts = make([]HostModel, 0)
		for _, item := range listData {
			if m, ok := item.(map[string]interface{}); ok {
				config.Hosts = append(config.Hosts, HostModel{
					ID:   types.StringValue(asIDString(m, "id")),
					Name: types.StringValue(asString(m, "name")),
					IP:   types.StringValue(asString(m, "ip")),
				})
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- HOSTS BY CUSTOMER ---

func NewHostsByCustomerDataSource() datasource.DataSource {
	return &hostsByCustomerDataSource{}
}

type hostsByCustomerDataSource struct {
	clientData *providerdata.ProviderData
}

type HostsByCustomerDSModel struct {
	ID         types.String `tfsdk:"id"`
	ProviderID types.Int64  `tfsdk:"provider_id"`
	HostID     types.Int64  `tfsdk:"host_id"`
	CustomerID types.String `tfsdk:"customer_id"`
	VpcID      types.String `tfsdk:"vpc_id"`
	PlanType   types.String `tfsdk:"plan_type"`
	Hosts      []HostModel  `tfsdk:"hosts"`
}

func (d *hostsByCustomerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_hosts_by_customer"
}

func (d *hostsByCustomerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *hostsByCustomerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing hosts by customer.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"provider_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Provider ID. Defaults to 7.",
			},
			"host_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Host ID. Defaults to provider config.",
			},
			"customer_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Customer ID. Defaults to provider config.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Defaults to provider config.",
			},
			"plan_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Plan type, defaults to 'k8s'.",
			},
			"hosts": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"ip": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *hostsByCustomerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config HostsByCustomerDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := d.clientData.CustomerID
	if !config.CustomerID.IsNull() && !config.CustomerID.IsUnknown() && config.CustomerID.ValueString() != "" {
		customerID = config.CustomerID.ValueString()
	}

	hostID := d.clientData.HostID
	if !config.HostID.IsNull() && !config.HostID.IsUnknown() {
		hostID = config.HostID.ValueInt64()
	}

	vpcID := d.clientData.DefaultVpcID
	if !config.VpcID.IsNull() && !config.VpcID.IsUnknown() && config.VpcID.ValueString() != "" {
		vpcID = config.VpcID.ValueString()
	}

	planType := "k8s"
	if !config.PlanType.IsNull() && !config.PlanType.IsUnknown() && config.PlanType.ValueString() != "" {
		planType = config.PlanType.ValueString()
	}

	providerID := int64(7)
	if !config.ProviderID.IsNull() && !config.ProviderID.IsUnknown() {
		providerID = config.ProviderID.ValueInt64()
	}

	payload := map[string]interface{}{
		"providerId":  providerID,
		"hostId":      hostID,
		"customer_id": customerID,
		"vpc_id":      vpcID,
		"planType":    planType,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathRegionHostsByCust, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listData []interface{}
	if err := json.Unmarshal(apiResp.Data, &listData); err == nil {
		config.ID = types.StringValue(fmt.Sprintf("%d-%d-%s-%s", providerID, hostID, vpcID, customerID))
		config.CustomerID = types.StringValue(customerID)
		config.ProviderID = types.Int64Value(providerID)
		config.HostID = types.Int64Value(hostID)
		config.VpcID = types.StringValue(vpcID)
		config.PlanType = types.StringValue(planType)
		config.Hosts = make([]HostModel, 0)
		for _, item := range listData {
			if m, ok := item.(map[string]interface{}); ok {
				config.Hosts = append(config.Hosts, HostModel{
					ID:   types.StringValue(asIDString(m, "id")),
					Name: types.StringValue(asString(m, "name")),
					IP:   types.StringValue(asString(m, "ip")),
				})
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}
