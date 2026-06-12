package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ resource.Resource                = (*VolumeResource)(nil)
	_ resource.ResourceWithConfigure   = (*VolumeResource)(nil)
	_ resource.ResourceWithImportState = (*VolumeResource)(nil)
)

const (
	volumeCreateTimeout = 25 * time.Minute
	volumeDeleteTimeout = 10 * time.Minute
)

// VolumeResource implements `viettelidc_volume`.
type VolumeResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type VolumeResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Size       types.Int64  `tfsdk:"size"`
	VolumeType types.String `tfsdk:"volume_type"`
	Status     types.String `tfsdk:"status"`
	VpcID      types.String `tfsdk:"vpc_id"`
}

func NewVolumeResource() resource.Resource { return &VolumeResource{} }

func (r *VolumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_volume"
}

func (r *VolumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Block Storage Volume.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"size": schema.Int64Attribute{
				Required:    true,
				Description: "Volume size in GiB.",
			},
			"volume_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Volume type, e.g. SSD / HDD.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *VolumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *VolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VolumeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"name":        plan.Name.ValueString(),
		"size":        plan.Size.ValueInt64(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if v := plan.VolumeType.ValueString(); v != "" {
		body["volume_type"] = v
	}

	apiResp, diags := callAPI(ctx, r.client, pathVolumeCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	volID, err := extractVolumeID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Volume create response missing id", err.Error())
		return
	}

	plan.ID = types.StringValue(volID)
	plan.VpcID = types.StringValue(vpcID)

	// Poll until AVAILABLE.
	pollBody := map[string]interface{}{
		"volume_id":   volID,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if err := pollForStatus(ctx, r.client, pathVolumeDetail, pollBody, "status", []string{"AVAILABLE", "success"}, volumeCreateTimeout); err != nil {
		resp.Diagnostics.AddError("Volume did not become AVAILABLE", err.Error())
		return
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VolumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.readInto(ctx, &state, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update handles size extension (shrinking is rejected).
func (r *VolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state VolumeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Size.ValueInt64() < state.Size.ValueInt64() {
		resp.Diagnostics.AddError(
			"Volume Shrink Not Allowed",
			fmt.Sprintf("cannot reduce volume size from %d GiB to %d GiB; only extension is supported.",
				state.Size.ValueInt64(), plan.Size.ValueInt64()),
		)
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"volume_id":   state.ID.ValueString(),
		"name":        plan.Name.ValueString(),
		"size":        plan.Size.ValueInt64(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if _, diags := callAPI(ctx, r.client, pathVolumeUpdate, body); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Wait until AVAILABLE again after resize.
	pollBody := map[string]interface{}{
		"volume_id":   state.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if err := pollForStatus(ctx, r.client, pathVolumeDetail, pollBody, "status", []string{"AVAILABLE", "IN-USE", "success"}, volumeCreateTimeout); err != nil {
		resp.Diagnostics.AddError("Volume did not return to AVAILABLE after update", err.Error())
		return
	}

	plan.ID = state.ID
	plan.VpcID = types.StringValue(vpcID)
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VolumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"volume_id":   state.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathVolumeDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	detailBody := map[string]interface{}{
		"volume_id":   state.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if err := pollUntilGone(ctx, r.client, pathVolumeDetail, detailBody, volumeDeleteTimeout); err != nil {
		resp.Diagnostics.AddError("Volume delete timeout", err.Error())
	}
}

func (r *VolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------- Helpers ----------

// readInto fetches /volume/detail and populates m. Returns false when the volume is gone.
func (r *VolumeResource) readInto(ctx context.Context, m *VolumeResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	body := map[string]interface{}{
		"volume_id":   m.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathVolumeDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	// Fail fast if volume entered a terminal error state.
	if apiResp != nil {
		var raw map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &raw); err == nil {
			if st := asString(raw, "status"); st == "error" || st == "failed" || st == "ERROR" || st == "FAILED" {
				diags.AddError(
					"Volume is in error state",
					fmt.Sprintf("Volume %s has status=%s. Destroy and re-create it before proceeding.", m.ID.ValueString(), st),
				)
				return true
			}
		}
	}
	if err := mapVolumeResponse(apiResp, m); err != nil {
		diags.AddError("Volume detail decode failed", err.Error())
	}
	return true
}

func extractVolumeID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "vttVolumeId"); id != "" {
		return id, nil
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("volume id not found in response: %s", string(resp.Data))
}

func mapVolumeResponse(resp *client.APIResponse, m *VolumeResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "vttVolumeId"); id != "" {
		m.ID = types.StringValue(id)
	} else if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	if v := asIDString(data, "size"); v != "" {
		if n, err := parseInt64(v); err == nil {
			m.Size = types.Int64Value(n)
		}
	} else if v, ok := data["size"].(float64); ok {
		m.Size = types.Int64Value(int64(v))
	}
	if v := asString(data, "volumeDisplayType"); v != "" {
		m.VolumeType = types.StringValue(v)
	} else if v := asString(data, "volumeType"); v != "" {
		m.VolumeType = types.StringValue(v)
	}
	m.Status = types.StringValue(asString(data, "status"))
	if vpcID := asIDString(data, "vpcId"); vpcID != "" {
		m.VpcID = types.StringValue(vpcID)
	}
	return nil
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
