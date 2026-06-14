package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

// CertificateDataSource looks up an existing certificate by ID or name.
type CertificateDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// CertificateDataSourceModel is the Terraform state for a certificate data source.
type CertificateDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	VpcID     types.String `tfsdk:"vpc_id"`
	Status    types.String `tfsdk:"status"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func NewCertificateDataSource() datasource.DataSource { return &CertificateDataSource{} }

func (d *CertificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_certificate"
}

func (d *CertificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing ViettelIDC TLS/SSL Certificate by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Certificate UUID. Either 'id' or 'name' must be provided.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Certificate display name. Either 'id' or 'name' must be provided.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current certificate status.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp (ISO-8601).",
			},
		},
	}
}

func (d *CertificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state CertificateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := defaultIfEmpty(state.VpcID, d.defaultVpcID)
	if vpcID == "" {
		resp.Diagnostics.AddError("Missing vpc_id", "Set 'vpc_id' or configure provider default.")
		return
	}
	if state.ID.ValueString() == "" && state.Name.ValueString() == "" {
		resp.Diagnostics.AddError("Missing filter",
			"Specify at least one of 'id' or 'name' to look up a certificate.")
		return
	}

	// GET /key-manager/api/v1/kms/{vpcId}/certificate/available returns only
	// {id, name}. Use the full list endpoint for status and createdAt.
	listPath := fmt.Sprintf("%s/%s/certificate", pathCertBase, vpcID)
	raw, diags := callKMS(ctx, d.client, http.MethodGet, listPath, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var listResp certListResp
	if err := json.Unmarshal(raw, &listResp); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	var found *certItem
	for i := range listResp.Items {
		item := &listResp.Items[i]
		if state.ID.ValueString() != "" && item.ID == state.ID.ValueString() {
			found = item
			break
		}
		if state.Name.ValueString() != "" && item.Name == state.Name.ValueString() {
			found = item
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError("Certificate not found",
			fmt.Sprintf("No certificate matching id=%q name=%q in VPC %s",
				state.ID.ValueString(), state.Name.ValueString(), vpcID))
		return
	}

	state.ID        = types.StringValue(found.ID)
	state.Name      = types.StringValue(found.Name)
	state.VpcID     = types.StringValue(vpcID)
	state.Status    = types.StringValue(found.Status)
	state.CreatedAt = types.StringValue(found.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
