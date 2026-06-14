package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ resource.Resource                = (*InstanceResource)(nil)
	_ resource.ResourceWithConfigure   = (*InstanceResource)(nil)
	_ resource.ResourceWithImportState = (*InstanceResource)(nil)
)

const (
	instanceCreateTimeout = 25 * time.Minute
	instanceDeleteTimeout = 20 * time.Minute
	instanceStopTimeout   = 5 * time.Minute
)

// InstanceResource implements `viettelidc_instance`.
type InstanceResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// InstanceResourceModel mirrors the resource schema.
type InstanceResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	TemplateID       types.Int64  `tfsdk:"template_id"`
	InstanceTypeID   types.Int64  `tfsdk:"instance_type_id"`
	AdminPass        types.String `tfsdk:"admin_pass"`
	CPU              types.Int64  `tfsdk:"cpu"`
	Memory           types.Int64  `tfsdk:"memory"`
	StorageType      types.String `tfsdk:"storage_type"`
	KeyPairName      types.String `tfsdk:"key_pair_name"`
	SubnetID         types.String `tfsdk:"subnet_id"`
	SecurityGroupIDs types.List   `tfsdk:"security_group_ids"`
	AvailabilityZone types.String `tfsdk:"availability_zone"`
	Status           types.String `tfsdk:"status"`
	IPAddress        types.String `tfsdk:"ip_address"`
	RootNicID        types.String `tfsdk:"root_nic_id"`
	ImageID          types.String `tfsdk:"image_id"`
	ImageName        types.String `tfsdk:"image_name"`
	VpcID            types.String `tfsdk:"vpc_id"`
}

func NewInstanceResource() resource.Resource { return &InstanceResource{} }

func (r *InstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_instance"
}

func (r *InstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Compute Instance (VM).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"template_id": schema.Int64Attribute{
				Required:    true,
				Description: "VM template (image) integer ID.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"instance_type_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Instance type (package) integer ID.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"admin_pass": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Initial admin password.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cpu": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Number of vCPUs.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"memory": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "RAM in MB.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"subnet_id": schema.StringAttribute{
				Required:    true,
				Description: "Subnet to attach the primary NIC to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"security_group_ids": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "List of Security Group IDs to attach.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"availability_zone": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Availability zone.",
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
			"ip_address": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"root_nic_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the primary NIC auto-created with this instance. Use this to associate a Floating IP without creating a separate NIC.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"image_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"image_name": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"storage_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Root volume storage type: \"SSD\" or \"HDD\". Defaults to \"HDD\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key_pair_name": schema.StringAttribute{
				Optional:    true,
				Description: "Key pair name to inject into the instance.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *InstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *InstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan InstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := r.buildCreateBody(ctx, &plan, vpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, diags := callAPI(ctx, r.client, pathVMCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmID, err := extractVMID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Instance create response missing id", err.Error())
		return
	}

	plan.ID = types.StringValue(vmID)
	plan.VpcID = types.StringValue(vpcID)

	// Poll until POWERED_ON (this VPC uses POWERED_ON; other VPCs may use ACTIVE).
	// The API Gateway renames instance_id→id before forwarding to API.
	pollBody := map[string]interface{}{
		"instance_id": vmID,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if err := pollForStatus(ctx, r.client, pathVMDetail, pollBody, "status", []string{"POWERED_ON", "ACTIVE"}, instanceCreateTimeout); err != nil {
		resp.Diagnostics.AddError("Instance did not become POWERED_ON/ACTIVE", err.Error())
		return
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *InstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state InstanceResourceModel
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

// Update handles resize (cpu/memory change). Requires stopping the VM first.
func (r *InstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state InstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	vmID := state.ID.ValueString()

	// Stop the VM first (id is sent as integer per HAR).
	vmIDInt, err := strconv.ParseInt(vmID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid VM ID", fmt.Sprintf("vm id %q is not a valid integer: %s", vmID, err))
		return
	}

	stopBody := map[string]interface{}{
		"instance_id": vmIDInt,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if _, diags := callAPI(ctx, r.client, pathVMStop, stopBody); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	pollBody := map[string]interface{}{
		"instance_id": vmID,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if err := pollForStatus(ctx, r.client, pathVMDetail, pollBody, "status", []string{"POWERED_OFF", "SHUTOFF"}, instanceStopTimeout); err != nil {
		resp.Diagnostics.AddError("Instance did not stop", err.Error())
		return
	}

	// Resize (The API Gateway renames instance_id→id).
	updateBody := map[string]interface{}{
		"instance_id": vmID,
		"cpu":         plan.CPU.ValueInt64(),
		"memory":      plan.Memory.ValueInt64(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if _, diags := callAPI(ctx, r.client, pathVMUpdate, updateBody); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Wait for ACTIVE again after resize.
	if err := pollForStatus(ctx, r.client, pathVMDetail, pollBody, "status", []string{"POWERED_ON", "ACTIVE"}, instanceCreateTimeout); err != nil {
		resp.Diagnostics.AddError("Instance did not become POWERED_ON/ACTIVE after resize", err.Error())
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

func (r *InstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state InstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	vmID := state.ID.ValueString()

	vmIDInt, err := strconv.ParseInt(vmID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid VM ID", fmt.Sprintf("vm id %q is not a valid integer: %s", vmID, err))
		return
	}

	// Stop the VM first so delete doesn't fail with ERROR_EXECUTE_THIS_ACTION.
	stopBody := map[string]interface{}{
		"instance_id": vmIDInt,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if stopResp, stopDiags := callAPI(ctx, r.client, pathVMStop, stopBody); stopDiags.HasError() {
		// Ignore stop errors if the VM is already stopped/gone.
		if stopResp == nil || !isNotFoundMessage(stopResp.Message) {
			// Best-effort: log but continue to delete attempt.
			_ = stopDiags
		}
	} else {
		// Wait until POWERED_OFF before deleting.
		pollStopBody := map[string]interface{}{
			"instance_id": vmID,
			"vpc_id":      vpcID,
			"customer_id": r.customerID,
		}
		_ = pollForStatus(ctx, r.client, pathVMDetail, pollStopBody, "status", []string{"POWERED_OFF", "SHUTOFF"}, instanceStopTimeout)
	}

	deleteBody := map[string]interface{}{
		"instance_id": vmIDInt,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathVMDelete, deleteBody)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until the VM is gone (The API Gateway renames instance_id→id).
	detailBody := map[string]interface{}{
		"instance_id": vmID,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if err := pollUntilGone(ctx, r.client, pathVMDetail, detailBody, instanceDeleteTimeout); err != nil {
		resp.Diagnostics.AddError("Instance delete timeout", err.Error())
	}
}

func (r *InstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------- Helpers ----------

func (r *InstanceResource) buildCreateBody(ctx context.Context, m *InstanceResourceModel, vpcID string) (map[string]interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := map[string]interface{}{
		"template_id": m.TemplateID.ValueInt64(),
		"subnet_id":   m.SubnetID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if v := m.AdminPass.ValueString(); v != "" {
		body["admin_pass"] = v
	}
	if v := m.AvailabilityZone.ValueString(); v != "" {
		body["availability_zone"] = v
	}

	// instanceType object — sent as-is (camelCase, no API Gateway rename).
	storageType := "HDD"
	if v := m.StorageType.ValueString(); v != "" {
		storageType = v
	}
	instanceType := map[string]interface{}{
		"storage":     20,
		"storageType": storageType,
	}
	if v := m.KeyPairName.ValueString(); v != "" {
		body["key_pair_name"] = v
	}
	if !m.InstanceTypeID.IsNull() && !m.InstanceTypeID.IsUnknown() && m.InstanceTypeID.ValueInt64() > 0 {
		instanceType["id"] = m.InstanceTypeID.ValueInt64()
	}
	if !m.CPU.IsNull() && !m.CPU.IsUnknown() {
		instanceType["cpu"] = m.CPU.ValueInt64()
	}
	if !m.Memory.IsNull() && !m.Memory.IsUnknown() {
		instanceType["memory"] = m.Memory.ValueInt64()
	}
	body["instanceType"] = instanceType

	if !m.SecurityGroupIDs.IsNull() && !m.SecurityGroupIDs.IsUnknown() {
		var sgIDs []string
		diags.Append(m.SecurityGroupIDs.ElementsAs(ctx, &sgIDs, false)...)
		if !diags.HasError() {
			sgs := make([]map[string]interface{}, 0, len(sgIDs))
			for _, id := range sgIDs {
				sgIDInt, _ := strconv.ParseInt(id, 10, 64)
				sgs = append(sgs, map[string]interface{}{"vttSecurityGroupId": sgIDInt})
			}
			body["securityGroups"] = sgs
		}
	}

	return body, diags
}

// readInto populates m from vm/detail. Returns false when VM is gone.
func (r *InstanceResource) readInto(ctx context.Context, m *InstanceResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	body := map[string]interface{}{
		"instance_id": m.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathVMDetail, body)
	if d.HasError() {
		// ERROR_VALIDATE_RESOURCE from vm/detail means the VM no longer exists (drift).
		if apiResp != nil && (isNotFoundMessage(apiResp.Message) || apiResp.Message == "ERROR_VALIDATE_RESOURCE") {
			return false
		}
		diags.Append(d...)
		return true
	}
	// Fail fast if instance entered a terminal error state.
	if apiResp != nil {
		var raw map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &raw); err == nil {
			if st := asString(raw, "status"); st == "error" || st == "failed" || st == "ERROR" || st == "FAILED" {
				diags.AddError(
					"Instance is in error state",
					fmt.Sprintf("Instance %s has status=%s. Destroy and re-create it before proceeding.", m.ID.ValueString(), st),
				)
				return true
			}
		}
	}
	if err := mapVMResponse(ctx, apiResp, m); err != nil {
		diags.AddError("Instance detail decode failed", err.Error())
	}
	return true
}

func extractVMID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("id not found in response: %s", string(resp.Data))
}

func mapVMResponse(ctx context.Context, resp *client.APIResponse, m *InstanceResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}

	if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	m.Status = types.StringValue(asString(data, "status"))
	// Preserve user-specified availability_zone: the API may return a different
	// alias (e.g. "AZ-PV-TIER-2" when "AZ-BD-TIER-2" was requested), which would
	// cause Terraform to report a provider inconsistency error. Only populate the
	// field from the API when no value is already present in the model.
	if m.AvailabilityZone.IsNull() || m.AvailabilityZone.IsUnknown() || m.AvailabilityZone.ValueString() == "" {
		m.AvailabilityZone = types.StringValue(asString(data, "availabilityZone"))
	}

	if vpcID := asIDString(data, "vpcId"); vpcID != "" {
		m.VpcID = types.StringValue(vpcID)
	}

	// Extract cpu/memory from vmEntity.
	if vmEntity, ok := data["vmEntity"].(map[string]interface{}); ok {
		if cpu := asIDString(vmEntity, "cpu"); cpu != "" {
			if n, err := strconv.ParseInt(cpu, 10, 64); err == nil {
				m.CPU = types.Int64Value(n)
			}
		}
		if mem := asIDString(vmEntity, "memory"); mem != "" {
			if n, err := strconv.ParseInt(mem, 10, 64); err == nil {
				m.Memory = types.Int64Value(n)
			}
		}
		if tid := asIDString(vmEntity, "vttTemplateId"); tid != "" {
			if n, err := strconv.ParseInt(tid, 10, 64); err == nil {
				m.TemplateID = types.Int64Value(n)
			}
		}
		if iid, ok := vmEntity["instanceTypeId"].(float64); ok && iid > 0 {
			m.InstanceTypeID = types.Int64Value(int64(iid))
		} else {
			m.InstanceTypeID = types.Int64Null()
		}
	}

	// Safety net: ensure InstanceTypeID is never left unknown after a read.
	if m.InstanceTypeID.IsUnknown() {
		m.InstanceTypeID = types.Int64Null()
	}

	// Extract image info.
	if image, ok := data["image"].(map[string]interface{}); ok {
		m.ImageID = types.StringValue(asIDString(image, "id"))
		m.ImageName = types.StringValue(asString(image, "name"))
	}

	// Extract primary IP and root NIC ID from networks[0].
	if networks, ok := data["networks"].([]interface{}); ok && len(networks) > 0 {
		if net, ok := networks[0].(map[string]interface{}); ok {
			m.IPAddress = types.StringValue(asString(net, "ipAddress"))
			if nicID := asIDString(net, "id"); nicID != "" {
				m.RootNicID = types.StringValue(nicID)
			}
		}
	}

	// Extract security group IDs — only when the model value is not already set
	// (i.e. null or unknown). When the user explicitly provides security_group_ids
	// (including an empty list []), we preserve that value to avoid plan inconsistency
	// caused by the API auto-assigning default security groups.
	if m.SecurityGroupIDs.IsNull() || m.SecurityGroupIDs.IsUnknown() {
		if sgs, ok := data["securityGroups"].([]interface{}); ok {
			sgIDs := make([]string, 0, len(sgs))
			for _, sg := range sgs {
				if sgMap, ok := sg.(map[string]interface{}); ok {
					if id := asIDString(sgMap, "vttSecurityGroupId"); id != "" {
						sgIDs = append(sgIDs, id)
					}
				}
			}
			listVal, diags := types.ListValueFrom(ctx, types.StringType, sgIDs)
			if !diags.HasError() {
				m.SecurityGroupIDs = listVal
			}
		}
	}

	return nil
}
