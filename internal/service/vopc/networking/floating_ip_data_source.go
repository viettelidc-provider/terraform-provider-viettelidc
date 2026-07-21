// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*FloatingIPDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*FloatingIPDataSource)(nil)
)

// FloatingIPDataSource implements `data "viettelidc_floating_ip"` (lookup by id).
type FloatingIPDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type FloatingIPDataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	PublicIP           types.String `tfsdk:"public_ip"`
	InstanceID         types.String `tfsdk:"instance_id"`
	NetworkInterfaceID types.String `tfsdk:"network_interface_id"`
	VpcID              types.String `tfsdk:"vpc_id"`
	Name               types.String `tfsdk:"name"`
	Type               types.String `tfsdk:"type"`
	Status             types.String `tfsdk:"status"`
	AttachmentStatus   types.String `tfsdk:"attachment_status"`
}

func NewFloatingIPDataSource() datasource.DataSource { return &FloatingIPDataSource{} }

func (d *FloatingIPDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_floating_ip"
}

func (d *FloatingIPDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ViettelIDC Floating IP by its ID (vttFloatingId) or Public IP.",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Optional: true, Computed: true, Description: "Floating IP ID (vttFloatingId). Mutually optional with public_ip."},
			"public_ip":            schema.StringAttribute{Optional: true, Computed: true, Description: "Public IPv4 address. Mutually optional with id."},
			"instance_id":          schema.StringAttribute{Computed: true, Description: "Associated VM instance ID."},
			"network_interface_id": schema.StringAttribute{Computed: true, Description: "Associated NIC ID."},
			"vpc_id":               schema.StringAttribute{Optional: true, Computed: true, Description: "VPC ID."},
			"name":                 schema.StringAttribute{Computed: true, Description: "Name of the Floating IP."},
			"type":                 schema.StringAttribute{Computed: true, Description: "Type of the Floating IP (e.g. public)."},
			"status":               schema.StringAttribute{Computed: true, Description: "Status of the Floating IP."},
			"attachment_status":    schema.StringAttribute{Computed: true, Description: "Attachment status (e.g. AVAILABLE)."},
		},
	}
}

func (d *FloatingIPDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *FloatingIPDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg FloatingIPDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasID := !cfg.ID.IsNull() && cfg.ID.ValueString() != ""
	hasPublicIP := !cfg.PublicIP.IsNull() && cfg.PublicIP.ValueString() != ""
	if !hasID && !hasPublicIP {
		resp.Diagnostics.AddError("Missing lookup key", "Set either 'id' or 'public_ip'.")
		return
	}

	vpcID := cfg.VpcID.ValueString()
	if vpcID == "" {
		vpcID = d.defaultVpcID
	}

	if hasID {
		body := map[string]interface{}{
			"floating_ip_id": cfg.ID.ValueString(),
			"vpc_id":         vpcID,
			"customer_id":    d.customerID,
		}
		apiResp, diags := callAPI(ctx, d.client, pathFloatingIPDetail, body)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		if err := mapFloatingIPDataSourceResponse(apiResp, &cfg); err != nil {
			resp.Diagnostics.AddError("Floating IP response decode failed", err.Error())
			return
		}
		cfg.VpcID = types.StringValue(vpcID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
		return
	}

	// Lookup by public_ip
	var vpcIdVal interface{} = vpcID
	if v, err := strconv.Atoi(vpcID); err == nil {
		vpcIdVal = v
	}

	body := map[string]interface{}{
		"pageIndex":  0,
		"pageSize":   100,
		"customer_id": d.customerID,
		"filters": []map[string]interface{}{
			{
				"name":   "vni.ip_address",
				"values": []string{cfg.PublicIP.ValueString()},
			},
		},
		"vpc_id": vpcIdVal,
	}
	apiResp, diags := callAPI(ctx, d.client, pathFloatingIPList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := decodeFloatingIPList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode floating ip list failed", err.Error())
		return
	}

	var found *FloatingIPDataSourceModel
	for i := range items {
		if items[i].PublicIP.ValueString() == cfg.PublicIP.ValueString() {
			found = &items[i]
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError("Floating IP not found", fmt.Sprintf("No Floating IP with public_ip %q found.", cfg.PublicIP.ValueString()))
		return
	}

	cfg.ID = found.ID
	cfg.InstanceID = found.InstanceID
	cfg.NetworkInterfaceID = found.NetworkInterfaceID
	cfg.VpcID = types.StringValue(vpcID)
	cfg.Name = found.Name
	cfg.Type = found.Type
	cfg.Status = found.Status
	cfg.AttachmentStatus = found.AttachmentStatus

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

func mapFloatingIPDataSourceResponse(resp *client.APIResponse, m *FloatingIPDataSourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	if ip := asString(data, "publicIp"); ip != "" {
		m.PublicIP = types.StringValue(ip)
	} else if ip := asString(data, "publicIP"); ip != "" {
		m.PublicIP = types.StringValue(ip)
	}
	if vmID := asIDString(data, "vttVmId"); vmID != "" {
		m.InstanceID = types.StringValue(vmID)
	}
	if nicID := asString(data, "vttNetworkInterfaceId"); nicID != "" {
		m.NetworkInterfaceID = types.StringValue(nicID)
	}
	m.Name = types.StringValue(asString(data, "name"))
	m.Type = types.StringValue(asString(data, "type"))
	m.Status = types.StringValue(asString(data, "status"))
	m.AttachmentStatus = types.StringValue(asString(data, "attachmentStatus"))
	return nil
}

func decodeFloatingIPList(resp *client.APIResponse) ([]FloatingIPDataSourceModel, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		var wrapper map[string]json.RawMessage
		if err2 := json.Unmarshal(resp.Data, &wrapper); err2 != nil {
			return nil, fmt.Errorf("decode floating ip list: %w", err2)
		}
		for _, key := range []string{"items", "data", "floatingIps", "vpcs"} {
			if v, ok := wrapper[key]; ok {
				if err3 := json.Unmarshal(v, &raw); err3 != nil {
					return nil, fmt.Errorf("decode floating ip list[%s]: %w", key, err3)
				}
				break
			}
		}
	}

	result := make([]FloatingIPDataSourceModel, 0, len(raw))
	for _, item := range raw {
		var data map[string]interface{}
		if err := json.Unmarshal(item, &data); err != nil {
			continue
		}
		var m FloatingIPDataSourceModel
		if id := asIDString(data, "vttFloatingId"); id != "" {
			m.ID = types.StringValue(id)
		} else if id := asIDString(data, "id"); id != "" {
			m.ID = types.StringValue(id)
		}
		
		if ip := asString(data, "publicIp"); ip != "" {
			m.PublicIP = types.StringValue(ip)
		} else if ip := asString(data, "publicIP"); ip != "" {
			m.PublicIP = types.StringValue(ip)
		} else if ip := asString(data, "floatingIp"); ip != "" {
			m.PublicIP = types.StringValue(ip)
		}

		if vmID := asIDString(data, "vttVmId"); vmID != "" {
			m.InstanceID = types.StringValue(vmID)
		}
		if nicID := asString(data, "vttNetworkInterfaceId"); nicID != "" {
			m.NetworkInterfaceID = types.StringValue(nicID)
		} else if nicID := asString(data, "networkInterfaceForFloating"); nicID != "" {
			m.NetworkInterfaceID = types.StringValue(nicID)
		}
		
		m.Name = types.StringValue(asString(data, "name"))
		m.Type = types.StringValue(asString(data, "type"))
		m.Status = types.StringValue(asString(data, "status"))
		m.AttachmentStatus = types.StringValue(asString(data, "attachmentStatus"))

		result = append(result, m)
	}
	return result, nil
}

