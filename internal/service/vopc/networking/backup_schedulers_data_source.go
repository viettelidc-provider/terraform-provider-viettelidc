// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*BackupSchedulersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*BackupSchedulersDataSource)(nil)
)

// BackupSchedulersDataSource implements `data "viettelidc_ovpc_backup_schedulers"`.
// Schedule ids are UUIDs assigned by the service, so listing is the only way to
// find one that Terraform did not create — for an import, say.
type BackupSchedulersDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type BackupSchedulersDataSourceModel struct {
	VpcID      types.String          `tfsdk:"vpc_id"`
	Schedulers []BackupSchedulerItem `tfsdk:"schedulers"`
}

type BackupSchedulerItem struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Cycle          types.String `tfsdk:"cycle"`
	StartDate      types.String `tfsdk:"start_date"`
	StartTime      types.String `tfsdk:"start_time"`
	NumberOfRecord types.Int64  `tfsdk:"number_of_record"`
	VMNumber       types.Int64  `tfsdk:"vm_number"`
	Status         types.String `tfsdk:"status"`
	NextTime       types.String `tfsdk:"next_time"`
}

func NewBackupSchedulersDataSource() datasource.DataSource { return &BackupSchedulersDataSource{} }

func (d *BackupSchedulersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_backup_schedulers"
}

func (d *BackupSchedulersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the backup schedules of a VPC.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Description: "VPC filter; falls back to the provider default vpc_id.",
			},
			"schedulers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.StringAttribute{Computed: true, Description: "Schedule UUID."},
						"name":             schema.StringAttribute{Computed: true},
						"cycle":            schema.StringAttribute{Computed: true},
						"start_date":       schema.StringAttribute{Computed: true},
						"start_time":       schema.StringAttribute{Computed: true},
						"number_of_record": schema.Int64Attribute{Computed: true},
						"vm_number":        schema.Int64Attribute{Computed: true, Description: "How many VMs the schedule covers."},
						"status":           schema.StringAttribute{Computed: true},
						"next_time":        schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *BackupSchedulersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *BackupSchedulersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg BackupSchedulersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.client.DoMethod(ctx, http.MethodGet, backupSchedulesPath(vpcID), nil)
	if err != nil {
		resp.Diagnostics.AddError("Cannot list backup schedules", err.Error())
		return
	}
	var env struct {
		Items []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Cycle          string `json:"cycle"`
			StartDate      string `json:"startDate"`
			StartTime      string `json:"startTime"`
			NumberOfRecord int64  `json:"numberOfRecord"`
			VMNumber       int64  `json:"vmNumber"`
			Status         string `json:"status"`
			NextTime       string `json:"nextTime"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		resp.Diagnostics.AddError("decode backup schedule list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.Schedulers = make([]BackupSchedulerItem, 0, len(env.Items))
	for _, it := range env.Items {
		cfg.Schedulers = append(cfg.Schedulers, BackupSchedulerItem{
			ID:             types.StringValue(it.ID),
			Name:           types.StringValue(it.Name),
			Cycle:          types.StringValue(it.Cycle),
			StartDate:      types.StringValue(it.StartDate),
			StartTime:      types.StringValue(it.StartTime),
			NumberOfRecord: types.Int64Value(it.NumberOfRecord),
			VMNumber:       types.Int64Value(it.VMNumber),
			Status:         types.StringValue(it.Status),
			NextTime:       types.StringValue(it.NextTime),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
