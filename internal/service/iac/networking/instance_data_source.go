package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ datasource.DataSource              = (*InstanceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*InstanceDataSource)(nil)
)

// InstanceDataSource implements `data "viettelidc_instance"`.
type InstanceDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type InstanceDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	CPU              types.Int64  `tfsdk:"cpu"`
	Memory           types.Int64  `tfsdk:"memory"`
	Status           types.String `tfsdk:"status"`
	IPAddress        types.String `tfsdk:"ip_address"`
	ImageID          types.String `tfsdk:"image_id"`
	ImageName        types.String `tfsdk:"image_name"`
	AvailabilityZone types.String `tfsdk:"availability_zone"`
	SecurityGroupIDs types.List   `tfsdk:"security_group_ids"`
	VpcID            types.String `tfsdk:"vpc_id"`
}

func NewInstanceDataSource() datasource.DataSource { return &InstanceDataSource{} }

func (d *InstanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_instance"
}

func (d *InstanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ViettelIDC Instance by id.",
		Attributes: map[string]schema.Attribute{
			"id":                schema.StringAttribute{Required: true},
			"name":              schema.StringAttribute{Computed: true},
			"cpu":               schema.Int64Attribute{Computed: true},
			"memory":            schema.Int64Attribute{Computed: true},
			"status":            schema.StringAttribute{Computed: true},
			"ip_address":        schema.StringAttribute{Computed: true},
			"image_id":          schema.StringAttribute{Computed: true},
			"image_name":        schema.StringAttribute{Computed: true},
			"availability_zone": schema.StringAttribute{Computed: true},
			"security_group_ids": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"vpc_id": schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

func (d *InstanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *InstanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg InstanceDataSourceModel
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
		"id":          cfg.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
	}
	apiResp, diags := callAPI(ctx, d.client, pathVMDetail, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		resp.Diagnostics.AddError("Instance detail decode failed", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	if id := asIDString(data, "id"); id != "" {
		cfg.ID = types.StringValue(id)
	}
	cfg.Name = types.StringValue(asString(data, "name"))
	cfg.Status = types.StringValue(asString(data, "status"))
	cfg.AvailabilityZone = types.StringValue(asString(data, "availabilityZone"))

	if vmEntity, ok := data["vmEntity"].(map[string]interface{}); ok {
		if cpu := asIDString(vmEntity, "cpu"); cpu != "" {
			if n, err := strconv.ParseInt(cpu, 10, 64); err == nil {
				cfg.CPU = types.Int64Value(n)
			}
		}
		if mem := asIDString(vmEntity, "memory"); mem != "" {
			if n, err := strconv.ParseInt(mem, 10, 64); err == nil {
				cfg.Memory = types.Int64Value(n)
			}
		}
	}

	if image, ok := data["image"].(map[string]interface{}); ok {
		cfg.ImageID = types.StringValue(asIDString(image, "id"))
		cfg.ImageName = types.StringValue(asString(image, "name"))
	}

	if networks, ok := data["networks"].([]interface{}); ok && len(networks) > 0 {
		if net, ok := networks[0].(map[string]interface{}); ok {
			cfg.IPAddress = types.StringValue(asString(net, "ipAddress"))
		}
	}

	if sgs, ok := data["securityGroups"].([]interface{}); ok {
		sgIDs := make([]string, 0, len(sgs))
		for _, sg := range sgs {
			if sgMap, ok := sg.(map[string]interface{}); ok {
				if id := asIDString(sgMap, "vttSecurityGroupId"); id != "" {
					sgIDs = append(sgIDs, id)
				}
			}
		}
		listVal, listDiags := types.ListValueFrom(ctx, types.StringType, sgIDs)
		resp.Diagnostics.Append(listDiags...)
		if !listDiags.HasError() {
			cfg.SecurityGroupIDs = listVal
		}
	} else {
		cfg.SecurityGroupIDs, _ = types.ListValueFrom(ctx, types.StringType, []string{})
	}

	// Check if we decoded correctly.
	if cfg.ID.IsNull() || cfg.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Instance Not Found", fmt.Sprintf("no instance with id %q", cfg.ID.ValueString()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
