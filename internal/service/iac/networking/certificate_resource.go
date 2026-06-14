package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

// certItem is one entry from GET /key-manager/api/v1/kms/{vpcId}/certificate.
type certItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// certListResp is the full response body from GET .../certificate.
type certListResp struct {
	PageIndex  int        `json:"pageIndex"`
	PageSize   int        `json:"pageSize"`
	TotalItems int        `json:"totalItems"`
	Items      []certItem `json:"items"`
}

// listCertificates calls GET /key-manager/api/v1/kms/{vpcId}/certificate.
// It is a package-level function so both resource and data source can share it.
func listCertificates(ctx context.Context, c *client.Client, vpcID string) ([]certItem, diag.Diagnostics) {
	listPath := fmt.Sprintf("%s/%s/certificate", pathCertBase, vpcID)
	raw, diags := callKMS(ctx, c, http.MethodGet, listPath, nil)
	if diags.HasError() {
		return nil, diags
	}
	var resp certListResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		diags.AddError("Parse Error", fmt.Sprintf("certificate list: %s", err.Error()))
		return nil, diags
	}
	return resp.Items, diags
}

// ─── Resource ────────────────────────────────────────────────────────────────

// CertificateResource manages viettelidc_certificate resources.
type CertificateResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// CertificateResourceModel is the Terraform state for a certificate.
type CertificateResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Certificate types.String `tfsdk:"certificate"`
	PrivateKey  types.String `tfsdk:"private_key"`
	VpcID       types.String `tfsdk:"vpc_id"`
	Status      types.String `tfsdk:"status"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func NewCertificateResource() resource.Resource { return &CertificateResource{} }

func (r *CertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_certificate"
}

func (r *CertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC TLS/SSL Certificate managed by the key-manager service.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Certificate UUID assigned by key-manager.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name of the certificate.",
			},
			"certificate": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "PEM-encoded certificate content (-----BEGIN CERTIFICATE-----).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"private_key": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "PEM-encoded private key (-----BEGIN RSA PRIVATE KEY----- or -----BEGIN PRIVATE KEY-----).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current certificate status: CREATING, SUCCESS, DELETING.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp (ISO-8601).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *CertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *CertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := defaultIfEmpty(plan.VpcID, r.defaultVpcID)
	if vpcID == "" {
		resp.Diagnostics.AddAttributeError(path.Root("vpc_id"), "Missing vpc_id",
			"Set 'vpc_id' or configure provider default.")
		return
	}
	plan.VpcID = types.StringValue(vpcID)

	vpcIDInt, err := strconv.Atoi(vpcID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid vpc_id", fmt.Sprintf("vpc_id must be numeric, got %q: %s", vpcID, err))
		return
	}
	customerIDInt, err := strconv.Atoi(r.customerID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid customer_id", fmt.Sprintf("provider customer_id must be numeric, got %q: %s", r.customerID, err))
		return
	}

	certPath := fmt.Sprintf("%s/%s/certificate", pathCertBase, vpcID)
	body := map[string]interface{}{
		"name":        plan.Name.ValueString(),
		"certificate": plan.Certificate.ValueString(),
		"privateKey":  plan.PrivateKey.ValueString(),
		"vpcId":       vpcIDInt,
		"customerId":  customerIDInt,
	}

	_, diags := callKMS(ctx, r.client, http.MethodPost, certPath, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Response is empty — poll list until the cert appears with status=SUCCESS.
	item, err := r.pollCertByName(ctx, vpcID, plan.Name.ValueString(), 5*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Certificate not ready", err.Error())
		return
	}

	plan.ID        = types.StringValue(item.ID)
	plan.Status    = types.StringValue(item.Status)
	plan.CreatedAt = types.StringValue(item.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *CertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	item, found, diags := r.readCertByID(ctx, state.VpcID.ValueString(), state.ID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	// Fail fast if certificate entered a terminal error state.
	if st := strings.ToUpper(item.Status); st == "ERROR" || st == "FAILED" {
		resp.Diagnostics.AddError(
			"Certificate is in error state",
			fmt.Sprintf("Certificate %s has status=%s. Destroy and re-create it before proceeding.", state.ID.ValueString(), item.Status),
		)
		return
	}

	state.Name      = types.StringValue(item.Name)
	state.Status    = types.StringValue(item.Status)
	state.CreatedAt = types.StringValue(item.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *CertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan CertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state CertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcIDInt, err := strconv.Atoi(plan.VpcID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid vpc_id", fmt.Sprintf("vpc_id must be a numeric string, got %q: %s", plan.VpcID.ValueString(), err))
		return
	}
	customerIDInt, err := strconv.Atoi(r.customerID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid customer_id", fmt.Sprintf("provider customer_id must be numeric, got %q: %s", r.customerID, err))
		return
	}

	certPath := fmt.Sprintf("%s/%s/certificate/%s", pathCertBase, plan.VpcID.ValueString(), state.ID.ValueString())
	body := map[string]interface{}{
		"name":       plan.Name.ValueString(),
		"vpcId":      vpcIDInt,
		"customerId": customerIDInt,
	}

	_, diags := callKMS(ctx, r.client, http.MethodPut, certPath, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-read state after update.
	item, found, readDiags := r.readCertByID(ctx, plan.VpcID.ValueString(), state.ID.ValueString())
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = state.ID
	if found {
		plan.Status    = types.StringValue(item.Status)
		plan.CreatedAt = types.StringValue(item.CreatedAt)
	} else {
		plan.Status    = state.Status
		plan.CreatedAt = state.CreatedAt
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *CertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcIDInt, err := strconv.Atoi(state.VpcID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid vpc_id", fmt.Sprintf("vpc_id must be a numeric string, got %q: %s", state.VpcID.ValueString(), err))
		return
	}
	customerIDInt, err := strconv.Atoi(r.customerID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid customer_id", fmt.Sprintf("provider customer_id must be numeric, got %q: %s", r.customerID, err))
		return
	}

	certPath := fmt.Sprintf("%s/%s/certificate/%s", pathCertBase, state.VpcID.ValueString(), state.ID.ValueString())
	body := map[string]interface{}{
		"vpcId":      vpcIDInt,
		"customerId": customerIDInt,
	}

	_, diags := callKMS(ctx, r.client, http.MethodDelete, certPath, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.pollCertGone(ctx, state.VpcID.ValueString(), state.ID.ValueString(), 5*time.Minute); err != nil {
		resp.Diagnostics.AddError("Delete timeout", err.Error())
	}
}

func (r *CertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: "vpcId/certId"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected format 'vpcId/certId', got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vpc_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (r *CertificateResource) readCertByID(ctx context.Context, vpcID, certID string) (certItem, bool, diag.Diagnostics) {
	items, diags := listCertificates(ctx, r.client, vpcID)
	if diags.HasError() {
		return certItem{}, false, diags
	}
	for _, item := range items {
		if item.ID == certID {
			return item, true, diags
		}
	}
	return certItem{}, false, diags
}

// pollCertByName polls GET list until a cert with matching name reaches SUCCESS.
func (r *CertificateResource) pollCertByName(ctx context.Context, vpcID, name string, timeout time.Duration) (certItem, error) {
	deadline := time.Now().Add(timeout)
	for {
		items, diags := listCertificates(ctx, r.client, vpcID)
		if !diags.HasError() {
			for _, item := range items {
				if item.Name == name {
					switch strings.ToUpper(item.Status) {
					case "SUCCESS":
						return item, nil
					case "ERROR", "FAILED":
						return certItem{}, fmt.Errorf("certificate creation failed (status=%s)", item.Status)
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return certItem{}, fmt.Errorf("timed out waiting for certificate %q to become SUCCESS", name)
		}
		select {
		case <-ctx.Done():
			return certItem{}, ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

// pollCertGone polls GET list until the cert ID is no longer present.
func (r *CertificateResource) pollCertGone(ctx context.Context, vpcID, certID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		items, diags := listCertificates(ctx, r.client, vpcID)
		if !diags.HasError() {
			found := false
			for _, item := range items {
				if item.ID == certID {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for certificate %s to be deleted", certID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}
