// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*RouteTableAssociationResource)(nil)
	_ resource.ResourceWithConfigure   = (*RouteTableAssociationResource)(nil)
	_ resource.ResourceWithImportState = (*RouteTableAssociationResource)(nil)
)

// RouteTableAssociationResource implements `viettelidc_route_table_association`.
// It attaches a subnet to a route table.
// Import ID format: "<route_table_id>/<subnet_id>"
type RouteTableAssociationResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type RouteTableAssociationResourceModel struct {
	ID           types.String `tfsdk:"id"`
	RouteTableID types.String `tfsdk:"route_table_id"`
	SubnetID     types.String `tfsdk:"subnet_id"`
	VpcID        types.String `tfsdk:"vpc_id"`
}

func NewRouteTableAssociationResource() resource.Resource {
	return &RouteTableAssociationResource{}
}

func (r *RouteTableAssociationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_route_table_association"
}

func (r *RouteTableAssociationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Associate a subnet with a ViettelIDC Route Table.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Composite ID: <route_table_id>/<subnet_id>",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"route_table_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subnet_id": schema.StringAttribute{
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

func (r *RouteTableAssociationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *RouteTableAssociationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RouteTableAssociationResourceModel
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
		"subnet_id":      plan.SubnetID.ValueString(),
		"route_table_id": plan.RouteTableID.ValueString(),
		"vpc_id":         vpcID,
		"customer_id":    r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathRouteTableSubnetAttach, body)
	if d.HasError() {
		if apiResp != nil && isAlreadyAttachedMessage(apiResp.Message) {
			// idempotent
		} else {
			resp.Diagnostics.Append(d...)
			return
		}
	}

	err := pollSubnetAssociation(ctx, r.client, plan.RouteTableID.ValueString(), plan.SubnetID.ValueString(), vpcID, r.customerID, true)
	if err != nil {
		resp.Diagnostics.AddWarning("Timeout waiting for subnet attach", err.Error())
		// Don't fail the creation completely if just timed out, maybe it's just slow
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", plan.RouteTableID.ValueString(), plan.SubnetID.ValueString()))
	plan.VpcID = types.StringValue(vpcID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read is a no-op: we keep state as-is (no single-association detail endpoint).
func (r *RouteTableAssociationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RouteTableAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: all attributes have RequiresReplace.
func (r *RouteTableAssociationResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *RouteTableAssociationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RouteTableAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"subnet_id":      state.SubnetID.ValueString(),
		"route_table_id": state.RouteTableID.ValueString(),
		"vpc_id":         vpcID,
		"customer_id":    r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathRouteTableSubnetDetach, body)
	if d.HasError() {
		if apiResp != nil && (isNotFoundMessage(apiResp.Message) || isNotAttachedMessage(apiResp.Message)) {
			return
		}
		resp.Diagnostics.Append(d...)
		return
	}

	err := pollSubnetAssociation(ctx, r.client, state.RouteTableID.ValueString(), state.SubnetID.ValueString(), vpcID, r.customerID, false)
	if err != nil {
		resp.Diagnostics.AddWarning("Timeout waiting for subnet detach", err.Error())
	}
}

func (r *RouteTableAssociationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: <route_table_id>/<subnet_id>")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("route_table_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subnet_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func getInt64(v interface{}) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case int64:
		return val
	case string:
		i, _ := strconv.ParseInt(val, 10, 64)
		return i
	}
	return 0
}

func getSubnetName(ctx context.Context, c *client.Client, subnetID, vpcID, customerID string) string {
	subnetIDInt, _ := strconv.ParseInt(subnetID, 10, 64)
	vpcIDInt, _ := strconv.ParseInt(vpcID, 10, 64)
	custIDInt, _ := strconv.ParseInt(customerID, 10, 64)

	body := map[string]interface{}{
		"subnet_id":   subnetIDInt,
		"vpc_id":      vpcIDInt,
		"customer_id": custIDInt,
	}

	apiResp, d := callAPI(ctx, c, pathSubnetDetail, body)
	if d.HasError() || apiResp == nil {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return ""
	}

	if name, ok := data["name"].(string); ok {
		return name
	}
	return ""
}

func pollSubnetAssociation(ctx context.Context, client *client.Client, routeTableID, subnetID, vpcID, customerID string, expectAttached bool) error {
	rtIDInt, _ := strconv.ParseInt(routeTableID, 10, 64)
	vpcIDInt, _ := strconv.ParseInt(vpcID, 10, 64)
	custIDInt, _ := strconv.ParseInt(customerID, 10, 64)
	subnetIDInt, _ := strconv.ParseInt(subnetID, 10, 64)

	subnetName := getSubnetName(ctx, client, subnetID, vpcID, customerID)

	body := map[string]interface{}{
		"pageIndex":       0,
		"pageSize":        1000,
		"filters":         []interface{}{},
		"vttRouteTableId": rtIDInt,
		"vpcId":           vpcIDInt,
		"customerId":      custIDInt,
	}

	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for subnet association status (expectAttached=%v)", expectAttached)
		case <-ticker.C:
			apiResp, d := callAPI(ctx, client, pathRouteTableSubnetList, body)
			if d.HasError() || apiResp == nil {
				continue
			}

			var respData struct {
				Items []map[string]interface{} `json:"items"`
			}
			if err := json.Unmarshal(apiResp.Data, &respData); err != nil {
				continue
			}

			found := false
			for _, item := range respData.Items {
				itemMatched := false

				// Match by name if we got it
				if subnetName != "" {
					if name, ok := item["name"].(string); ok && name == subnetName {
						itemMatched = true
					}
				}

				// Fallback ID matching just in case
				if !itemMatched {
					id1 := getInt64(item["id"])
					id2 := getInt64(item["subnetId"])
					id3 := getInt64(item["vttSubnetId"])
					if id1 == subnetIDInt || id2 == subnetIDInt || id3 == subnetIDInt {
						itemMatched = true
					} else {
						itemBytes, _ := json.Marshal(item)
						if strings.Contains(string(itemBytes), subnetID) {
							itemMatched = true
						}
					}
				}

				if itemMatched {
					status, _ := item["status"].(string)
					if status == "ATTACHED" {
						found = true
						break
					}
				}
			}

			if expectAttached && found {
				return nil
			}
			if !expectAttached && !found {
				return nil
			}
		}
	}
}
