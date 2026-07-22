// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*BackupVMsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*BackupVMsDataSource)(nil)
)

// BackupVMsDataSource implements `data "viettelidc_ovpc_backup_vms"`.
//
// viettelidc_ovpc_backup_scheduler takes the backup service's own VM UUIDs,
// which are not the numeric instance ids. Without this data source the only way
// to learn them is to call the API by hand.
type BackupVMsDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type BackupVMsDataSourceModel struct {
	VpcID  types.String   `tfsdk:"vpc_id"`
	Filter types.String   `tfsdk:"filter"`
	VMs    []BackupVMItem `tfsdk:"vms"`
}

type BackupVMItem struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func NewBackupVMsDataSource() datasource.DataSource { return &BackupVMsDataSource{} }

func (d *BackupVMsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_backup_vms"
}

func (d *BackupVMsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the VMs the backup service offers for a VPC, with the UUIDs that " +
			"viettelidc_ovpc_backup_scheduler expects in vm_ids.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Description: "VPC filter; falls back to the provider default vpc_id.",
			},
			"filter": schema.StringAttribute{
				Optional: true,
				Description: `Which VMs to return: "all" (default) or "unassigned" — ` +
					"those not yet a member of any backup schedule.",
				Validators: []validator.String{
					stringvalidator.OneOf("all", "unassigned"),
				},
			},
			"vms": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, Description: "VM UUID, as used in vm_ids."},
						"name": schema.StringAttribute{Computed: true, Description: "VM name."},
					},
				},
			},
		},
	}
}

func (d *BackupVMsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *BackupVMsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg BackupVMsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := cfg.Filter.ValueString()
	if filter == "" {
		filter = "all"
	}

	raw, err := d.client.DoMethod(ctx, http.MethodGet,
		fmt.Sprintf("/backup/api/v1/vpc/%s/vms/%s", vpcID, filter), nil)
	if err != nil {
		resp.Diagnostics.AddError("Cannot list VMs available for backup", err.Error())
		return
	}
	var env struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		resp.Diagnostics.AddError("decode backup VM list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.Filter = types.StringValue(filter)
	cfg.VMs = make([]BackupVMItem, 0, len(env.Items))
	for _, it := range env.Items {
		cfg.VMs = append(cfg.VMs, BackupVMItem{
			ID:   types.StringValue(it.ID),
			Name: types.StringValue(it.Name),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
