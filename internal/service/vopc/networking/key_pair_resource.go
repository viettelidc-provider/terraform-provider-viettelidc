// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*KeyPairResource)(nil)
	_ resource.ResourceWithConfigure   = (*KeyPairResource)(nil)
	_ resource.ResourceWithImportState = (*KeyPairResource)(nil)
)

// KeyPairResource implements `viettelidc_key_pair`.
//
// CSA does not expose a single-keypair detail endpoint; Read is done via
// the paging list filtered by key_name.
type KeyPairResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type KeyPairResourceModel struct {
	ID          types.String `tfsdk:"id"`
	KeyName     types.String `tfsdk:"key_name"`
	DownloadURL types.String `tfsdk:"download_url"`
	VpcID       types.String `tfsdk:"vpc_id"`
}

func NewKeyPairResource() resource.Resource { return &KeyPairResource{} }

func (r *KeyPairResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_key_pair"
}

func (r *KeyPairResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Key Pair — SSH key pair for VM access.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key_name": schema.StringAttribute{
				Required:    true,
				Description: "Key pair name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"download_url": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "URL to download the private key file. Available immediately after creation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Falls back to provider default when unset.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *KeyPairResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *KeyPairResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan KeyPairResourceModel
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
		"key_name":    plan.KeyName.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathKeyPairCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	kp, err := decodeKeyPairResponse(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Key Pair create response decode failed", err.Error())
		return
	}

	plan.ID = types.StringValue(kp.id)
	plan.VpcID = types.StringValue(vpcID)
	plan.DownloadURL = types.StringValue(kp.downloadURL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *KeyPairResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state KeyPairResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathKeyPairList, body)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	items, err := decodeKeyPairList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Key Pair list decode failed", err.Error())
		return
	}

	targetName := state.KeyName.ValueString()
	for _, item := range items {
		if asString(item, "name") == targetName {
			if id := asIDString(item, "id"); id != "" {
				state.ID = types.StringValue(id)
			}
			state.VpcID = types.StringValue(vpcID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	// Key pair not found — remove from state.
	resp.State.RemoveResource(ctx)
}

// Update is a no-op (key_name has RequiresReplace).
func (r *KeyPairResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *KeyPairResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state KeyPairResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"keypair_id":  state.ID.ValueString(),
		"vpc_id":      state.VpcID.ValueString(),
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathKeyPairDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
	}
}

func (r *KeyPairResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------- Helpers ----------

type keyPairFields struct {
	id          string
	name        string
	downloadURL string
}

func decodeKeyPairResponse(resp *client.APIResponse) (*keyPairFields, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	kp := &keyPairFields{}
	kp.id = asIDString(data, "id")
	if kp.id == "" {
		kp.id = asIDString(data, "keypairId")
	}
	if kp.id == "" {
		return nil, fmt.Errorf("id not found in response: %s", string(resp.Data))
	}
	kp.name = asString(data, "name")
	kp.downloadURL = asString(data, "downloadUrl")
	if kp.downloadURL == "" {
		kp.downloadURL = asString(data, "privateKeyDownloadUrl")
	}
	return kp, nil
}

func decodeKeyPairList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	var raw interface{}
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	if arr, ok := raw.([]interface{}); ok {
		return toMapSlice(arr), nil
	}
	if m, ok := raw.(map[string]interface{}); ok {
		if content, ok := m["content"].([]interface{}); ok {
			return toMapSlice(content), nil
		}
		if list, ok := m["data"].([]interface{}); ok {
			return toMapSlice(list), nil
		}
		if items, ok := m["items"].([]interface{}); ok {
			return toMapSlice(items), nil
		}
	}
	return nil, fmt.Errorf("unexpected list structure: %T", raw)
}
