package networking

import (
"context"
"fmt"
"strings"

"github.com/hashicorp/terraform-plugin-framework/path"
"github.com/hashicorp/terraform-plugin-framework/resource"
"github.com/hashicorp/terraform-plugin-framework/resource/schema"
"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
"github.com/hashicorp/terraform-plugin-framework/types"

"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
_ resource.Resource                = (*VolumeAttachmentResource)(nil)
_ resource.ResourceWithConfigure   = (*VolumeAttachmentResource)(nil)
_ resource.ResourceWithImportState = (*VolumeAttachmentResource)(nil)
)

// VolumeAttachmentResource implements `viettelidc_volume_attachment`.
// The composite state ID is "<instance_id>/<volume_id>".
type VolumeAttachmentResource struct {
client       *client.Client
customerID   string
defaultVpcID string
}

type VolumeAttachmentResourceModel struct {
ID         types.String `tfsdk:"id"`
InstanceID types.String `tfsdk:"instance_id"`
VolumeID   types.String `tfsdk:"volume_id"`
VpcID      types.String `tfsdk:"vpc_id"`
}

func NewVolumeAttachmentResource() resource.Resource { return &VolumeAttachmentResource{} }

func (r *VolumeAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
resp.TypeName = req.ProviderTypeName + "_ovpc_volume_attachment"
}

func (r *VolumeAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
resp.Schema = schema.Schema{
Description: "Attach a ViettelIDC Block Storage Volume to a Compute Instance.",
Attributes: map[string]schema.Attribute{
"id": schema.StringAttribute{
Computed:    true,
Description: "Composite ID: <instance_id>/<volume_id>",
PlanModifiers: []planmodifier.String{
stringplanmodifier.UseStateForUnknown(),
},
},
"instance_id": schema.StringAttribute{
Required: true,
PlanModifiers: []planmodifier.String{
stringplanmodifier.RequiresReplace(),
},
},
"volume_id": schema.StringAttribute{
Required: true,
PlanModifiers: []planmodifier.String{
stringplanmodifier.RequiresReplace(),
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

func (r *VolumeAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
pd, diags := providerDataFrom(req.ProviderData)
resp.Diagnostics.Append(diags...)
if pd == nil {
return
}
r.client = pd.Client
r.customerID = pd.CustomerID
r.defaultVpcID = pd.DefaultVpcID
}

func (r *VolumeAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
var plan VolumeAttachmentResourceModel
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
"instance_id": plan.InstanceID.ValueString(),
"volume_id":   plan.VolumeID.ValueString(),
"vpc_id":      vpcID,
"customer_id": r.customerID,
}
apiResp, d := callAPI(ctx, r.client, pathVolumeAttach, body)
if d.HasError() {
if apiResp != nil && isAlreadyAttachedMessage(apiResp.Message) {
// idempotent: already attached is fine
} else {
resp.Diagnostics.Append(d...)
return
}
}

plan.ID = types.StringValue(fmt.Sprintf("%s/%s", plan.InstanceID.ValueString(), plan.VolumeID.ValueString()))
plan.VpcID = types.StringValue(vpcID)
resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read is a no-op: there is no attachment detail endpoint.
func (r *VolumeAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
var state VolumeAttachmentResourceModel
resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
if resp.Diagnostics.HasError() {
return
}
resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: all attributes have RequiresReplace.
func (r *VolumeAttachmentResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *VolumeAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
var state VolumeAttachmentResourceModel
resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
if resp.Diagnostics.HasError() {
return
}

vpcID := state.VpcID.ValueString()
if vpcID == "" {
vpcID = r.defaultVpcID
}

body := map[string]interface{}{
"instance_id": state.InstanceID.ValueString(),
"volume_id":   state.VolumeID.ValueString(),
"vpc_id":      vpcID,
"customer_id": r.customerID,
}
apiResp, d := callAPI(ctx, r.client, pathVolumeDetach, body)
if d.HasError() {
if apiResp != nil && (isNotFoundMessage(apiResp.Message) || isNotAttachedMessage(apiResp.Message)) {
return
}
resp.Diagnostics.Append(d...)
}
}

func (r *VolumeAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
parts := strings.SplitN(req.ID, "/", 2)
if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
resp.Diagnostics.AddError("Invalid import ID", "Expected format: <instance_id>/<volume_id>")
return
}
resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), parts[0])...)
resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("volume_id"), parts[1])...)
resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func isAlreadyAttachedMessage(msg string) bool {
m := strings.ToLower(msg)
return strings.Contains(m, "already attached") || strings.Contains(m, "already exists")
}
