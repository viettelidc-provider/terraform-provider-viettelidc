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
	_ datasource.DataSource = (*BackupRecordDataSource)(nil)
)

type BackupRecordDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type BackupRecordDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	BackupPlanID types.String `tfsdk:"backup_plan_id"`
	VolumeID     types.String `tfsdk:"volume_id"`
	VolumeName   types.String `tfsdk:"volume_name"`
	VolumeSize   types.Int64  `tfsdk:"volume_size"`
	Status       types.String `tfsdk:"status"`
	CreatedAt    types.String `tfsdk:"created_at"`
	VpcID        types.String `tfsdk:"vpc_id"`
}

func NewBackupRecordDataSource() datasource.DataSource { return &BackupRecordDataSource{} }

func (d *BackupRecordDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_backup_record"
}

func (d *BackupRecordDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lookup a Backup Record by ID in a VPC.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Backup Record ID.",
			},
			"backup_plan_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Filter by Backup Plan ID.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID to search within. Uses provider default if not specified.",
			},
			"volume_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the volume that was backed up.",
			},
			"volume_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the volume that was backed up.",
			},
			"volume_size": schema.Int64Attribute{
				Computed:    true,
				Description: "Size of the volume that was backed up.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Status of the backup record.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Timestamp when the backup was created.",
			},
		},
	}
}

func (d *BackupRecordDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *BackupRecordDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BackupRecordDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := defaultIfEmpty(config.VpcID, d.defaultVpcID)
	if vpcID == "" {
		resp.Diagnostics.AddError("Missing vpc_id", "Set 'vpc_id' or configure provider default.")
		return
	}

	if config.ID.IsNull() {
		resp.Diagnostics.AddError("Missing filter", "'id' must be specified.")
		return
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
		"page_index":  0,
		"page_size":   1000,
		"filters":     []map[string]interface{}{},
	}

	apiResp, diags := callAPI(ctx, d.client, pathBackupRecordList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var listResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Content []struct {
				ID           int64  `json:"id"`
				BackupPlanID int64  `json:"vttBackupPlanId"`
				VolumeID     int64  `json:"vttVolumeId"`
				VolumeName   string `json:"volumeName"`
				VolumeSize   int    `json:"volumeSize"`
				Status       string `json:"status"`
				CreatedAt    int64  `json:"createdAt"`
			} `json:"content"`
		} `json:"data"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	var found *struct {
		ID           int64  `json:"id"`
		BackupPlanID int64  `json:"vttBackupPlanId"`
		VolumeID     int64  `json:"vttVolumeId"`
		VolumeName   string `json:"volumeName"`
		VolumeSize   int    `json:"volumeSize"`
		Status       string `json:"status"`
		CreatedAt    int64  `json:"createdAt"`
	}

	for _, item := range listResp.Data.Content {
		if fmt.Sprintf("%d", item.ID) == config.ID.ValueString() {
			found = &item
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Backup Record %s not found", config.ID.ValueString()))
		return
	}

	result := BackupRecordDataSourceModel{
		ID:           types.StringValue(fmt.Sprintf("%d", found.ID)),
		BackupPlanID: types.StringValue(fmt.Sprintf("%d", found.BackupPlanID)),
		VpcID:        types.StringValue(vpcID),
		VolumeID:     types.StringValue(fmt.Sprintf("%d", found.VolumeID)),
		VolumeName:   types.StringValue(found.VolumeName),
		VolumeSize:   types.Int64Value(int64(found.VolumeSize)),
		Status:       types.StringValue(found.Status),
		CreatedAt:    types.StringValue(fmt.Sprintf("%d", found.CreatedAt)),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}
