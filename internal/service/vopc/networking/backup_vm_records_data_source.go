// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*BackupVMRecordsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*BackupVMRecordsDataSource)(nil)
)

// backupVMRecordsPageSize is what one page request asks for. The endpoint
// paginates and reports totalItems, so Read walks every page rather than
// exposing page/size and letting a config silently see only the first slice.
const backupVMRecordsPageSize = 100

// BackupVMRecordsDataSource implements `data "viettelidc_ovpc_backup_vm_records"`.
//
// These are the backups viettelidc_ovpc_backup_scheduler produces — whole VMs.
// data.viettelidc_ovpc_backup_record is the volume-backup equivalent belonging
// to viettelidc_ovpc_backup_plan; the two services are separate and their
// records do not overlap.
type BackupVMRecordsDataSource struct {
	client       *client.Client
	defaultVpcID string
}

type BackupVMRecordsDataSourceModel struct {
	VpcID   types.String         `tfsdk:"vpc_id"`
	Records []BackupVMRecordItem `tfsdk:"records"`
}

// backupVMRecordEntry is one element of the backup-records page envelope.
type backupVMRecordEntry struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	VMName       string  `json:"vmName"`
	VMID         string  `json:"vmId"`
	ScheduleName *string `json:"scheduleName"`
	Size         int64   `json:"size"`
	Status       string  `json:"status"`
	CreatedDate  string  `json:"createdDate"`
}

type BackupVMRecordItem struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	VMName       types.String `tfsdk:"vm_name"`
	VMID         types.String `tfsdk:"vm_id"`
	ScheduleName types.String `tfsdk:"schedule_name"`
	Size         types.Int64  `tfsdk:"size"`
	Status       types.String `tfsdk:"status"`
	CreatedDate  types.String `tfsdk:"created_date"`
}

func NewBackupVMRecordsDataSource() datasource.DataSource { return &BackupVMRecordsDataSource{} }

func (d *BackupVMRecordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_backup_vm_records"
}

func (d *BackupVMRecordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the VM backups a VPC holds, newest first — the records produced by " +
			"viettelidc_ovpc_backup_scheduler. For volume backups (viettelidc_ovpc_backup_plan) " +
			"use viettelidc_ovpc_backup_record instead.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Description: "VPC filter; falls back to the provider default vpc_id.",
			},
			"records": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{Computed: true, Description: "Backup record UUID."},
						"name":    schema.StringAttribute{Computed: true, Description: "Backup record name."},
						"vm_name": schema.StringAttribute{Computed: true, Description: "Name of the VM that was backed up."},
						"vm_id": schema.StringAttribute{
							Computed:    true,
							Description: "Backup-service UUID of the VM, the same id viettelidc_ovpc_backup_vms returns.",
						},
						"schedule_name": schema.StringAttribute{
							Computed:    true,
							Description: "Schedule that created the record; empty for a manual backup.",
						},
						"size":         schema.Int64Attribute{Computed: true, Description: "Backup size in bytes."},
						"status":       schema.StringAttribute{Computed: true, Description: `Record status, e.g. "AVAILABLE".`},
						"created_date": schema.StringAttribute{Computed: true, Description: "Creation time, RFC 3339."},
					},
				},
			},
		},
	}
}

func (d *BackupVMRecordsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.defaultVpcID = pd.DefaultVpcID
}

// fetchAll walks every page of the backup-records endpoint. It is a method of
// its own so the paging can be tested without standing up a ReadRequest.
func (d *BackupVMRecordsDataSource) fetchAll(ctx context.Context, vpcID string) ([]backupVMRecordEntry, error) {
	var entries []backupVMRecordEntry
	for page := 0; ; page++ {
		path := fmt.Sprintf("/backup/api/v1/vpc/%s/backup-records?page=%d&size=%d&sort=createdDate,desc",
			vpcID, page, backupVMRecordsPageSize)
		raw, err := d.client.DoMethod(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var env struct {
			Items      []backupVMRecordEntry `json:"items"`
			Data       []backupVMRecordEntry `json:"data"`
			TotalItems int                   `json:"totalItems"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode VM backup record list: %w", err)
		}
		// Sibling endpoints under this service disagree on items vs data, so
		// accept either here too.
		batch := env.Items
		if len(batch) == 0 {
			batch = env.Data
		}
		entries = append(entries, batch...)
		// Stop on a short or empty page: totalItems alone would spin forever if
		// the server ever reports a count it will not hand out.
		if len(batch) < backupVMRecordsPageSize || len(entries) >= env.TotalItems {
			return entries, nil
		}
	}
}

func (d *BackupVMRecordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg BackupVMRecordsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	entries, err := d.fetchAll(ctx, vpcID)
	if err != nil {
		resp.Diagnostics.AddError("Cannot list VM backup records", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.Records = make([]BackupVMRecordItem, 0, len(entries))
	for _, it := range entries {
		schedule := ""
		if it.ScheduleName != nil {
			schedule = *it.ScheduleName
		}
		cfg.Records = append(cfg.Records, BackupVMRecordItem{
			ID:           types.StringValue(it.ID),
			Name:         types.StringValue(it.Name),
			VMName:       types.StringValue(it.VMName),
			VMID:         types.StringValue(it.VMID),
			ScheduleName: types.StringValue(schedule),
			Size:         types.Int64Value(it.Size),
			Status:       types.StringValue(it.Status),
			CreatedDate:  types.StringValue(it.CreatedDate),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
