// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*VDBSParameterGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*VDBSParameterGroupResource)(nil)
	_ resource.ResourceWithImportState = (*VDBSParameterGroupResource)(nil)
)

// VDBSParameterGroupResource implements `viettelidc_vdbs_parameter_group`.
type VDBSParameterGroupResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// VDBSParameterGroupResourceModel mirrors the resource schema.
type VDBSParameterGroupResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Family      types.String `tfsdk:"family"`
	Description types.String `tfsdk:"description"`
	VpcID       types.String `tfsdk:"vpc_id"`
	Parameters  types.List   `tfsdk:"parameter"`
	InstanceID  types.String `tfsdk:"instance_id"`
}

// ParameterModel represents a single parameter nested block.
type ParameterModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

var parameterAttrTypes = map[string]attr.Type{
	"name":  types.StringType,
	"value": types.StringType,
}

// parameterEntry is the JSON wire format used in API requests/responses.
type parameterEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// NewVDBSParameterGroupResource constructs the resource.
func NewVDBSParameterGroupResource() resource.Resource {
	return &VDBSParameterGroupResource{}
}

func (r *VDBSParameterGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_parameter_group"
}

func (r *VDBSParameterGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC VDBS Parameter Group — configures database engine parameters reusable across DB instances.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Parameter group ID assigned by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Parameter group name.",
			},
			"family": schema.StringAttribute{
				Required:    true,
				Description: "DB engine family (e.g. mysql8.0, postgres14).",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional description.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parameter": schema.ListNestedAttribute{
				Required:    true,
				Description: "List of database engine parameters.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Parameter name.",
						},
						"value": schema.StringAttribute{
							Required:    true,
							Description: "Parameter value.",
						},
					},
				},
			},
			"instance_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Database instance ID to attach this parameter group to.",
			},
		},
	}
}

func (r *VDBSParameterGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *VDBSParameterGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VDBSParameterGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve host ID from VPC
	hostID, err := resolveHostID(ctx, r.client, r.customerID, vpcID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve region host ID", err.Error())
		return
	}

	// Resolve datastore ID & datastore version ID
	datastoreID, versionID, err := resolveEngineVersion(ctx, r.client, plan.Family.ValueString(), hostID, r.customerID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve datastore version ID", err.Error())
		return
	}

	vpcIDInt, _ := strconv.Atoi(vpcID)
	body := map[string]interface{}{
		"vpcId":                vpcIDInt,
		"name":                 plan.Name.ValueString(),
		"datastoreId":          datastoreID,
		"datastoreVersionId":   versionID,
		"datastore_version_id": nil,
		"hostId":               hostID,
		"customerId":           r.customerID,
		"planType":             "dbs",
	}
	if d := plan.Description.ValueString(); d != "" {
		body["description"] = d
	}

	_, callDiags := callDBSAPI(ctx, r.client, http.MethodPost, pathParamGroupCreate, body)
	resp.Diagnostics.Append(callDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query list to find group ID by name
	listPath := fmt.Sprintf("%s?pageIndex=0&pageSize=100&hostId=%d", pathParamGroupList, hostID)
	listResp, callDiags := callDBSAPI(ctx, r.client, http.MethodGet, listPath, nil)
	resp.Diagnostics.Append(callDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(listResp.Data, &listData); err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("failed to parse parameter group list: %s", err))
		return
	}

	var pgID string
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if asString(itemMap, "name") == plan.Name.ValueString() {
						pgID = asIDString(itemMap, "id")
						break
					}
				}
			}
		}
	}

	if pgID == "" {
		resp.Diagnostics.AddError("Created Parameter Group not found", fmt.Sprintf("could not find created group with name %q in region host %d", plan.Name.ValueString(), hostID))
		return
	}

	plan.ID = types.StringValue(pgID)
	plan.VpcID = types.StringValue(vpcID)

	// Update parameters if any configured
	r.updateParameters(ctx, pgID, hostID, plan.Parameters, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Attach to DB instance if instance_id is specified
	if !plan.InstanceID.IsNull() && !plan.InstanceID.IsUnknown() && plan.InstanceID.ValueString() != "" {
		serviceInit, err := resolveServiceInit(ctx, r.client, r.customerID, plan.InstanceID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to resolve database instance", err.Error())
			return
		}

		attachBody := map[string]interface{}{
			"serviceInitId":  serviceInit,
			"groupId":        pgID,
			"rebootRequired": true,
			"hostId":         hostID,
			"customerId":     r.customerID,
			"planType":       "dbs",
		}

		_, attachDiags := callDBSAPI(ctx, r.client, http.MethodPost, pathParamGroupAttach, attachBody)
		resp.Diagnostics.Append(attachDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else if plan.InstanceID.IsUnknown() {
		plan.InstanceID = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VDBSParameterGroupResource) updateParameters(ctx context.Context, pgID string, hostID int, planParams types.List, diags *diag.Diagnostics) {
	if planParams.IsNull() || planParams.IsUnknown() {
		return
	}
	var params []ParameterModel
	d := planParams.ElementsAs(ctx, &params, false)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	if len(params) == 0 {
		return
	}

	// 1. Fetch available parameters to resolve data types
	paramPath := fmt.Sprintf(pathParamGroupParameters, pgID) + "?pageIndex=0&pageSize=1000"
	paramResp, callDiags := callDBSAPI(ctx, r.client, http.MethodGet, paramPath, nil)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var paramData map[string]interface{}
	if err := json.Unmarshal(paramResp.Data, &paramData); err != nil {
		diags.AddError("Parse Error", fmt.Sprintf("failed to parse parameter list response: %s", err))
		return
	}

	typeMap := make(map[string]string)
	if itemsRaw, ok := paramData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					name := asString(itemMap, "name")
					dataType := asString(itemMap, "dataType")
					if name != "" {
						typeMap[name] = dataType
					}
				}
			}
		}
	}

	// 2. Build parameter map with casted values
	paramMap := make(map[string]interface{})
	for _, p := range params {
		name := p.Name.ValueString()
		valStr := p.Value.ValueString()
		dataType := typeMap[name]

		switch dataType {
		case "numeric":
			if valInt, err := strconv.Atoi(valStr); err == nil {
				paramMap[name] = valInt
			} else if valFloat, err := strconv.ParseFloat(valStr, 64); err == nil {
				paramMap[name] = valFloat
			} else {
				paramMap[name] = valStr
			}
		case "boolean":
			if valStr == "true" || valStr == "1" {
				paramMap[name] = true
			} else if valStr == "false" || valStr == "0" {
				paramMap[name] = false
			} else {
				paramMap[name] = valStr
			}
		default:
			paramMap[name] = valStr
		}
	}

	body := map[string]interface{}{
		"groupId":            pgID,
		"parameters":         paramMap,
		"instancesRebootNow": []interface{}{},
		"hostId":             hostID,
		"customerId":         r.customerID,
		"planType":           "dbs",
	}

	_, callDiags = callDBSAPI(ctx, r.client, http.MethodPut, pathParamGroupUpdate, body)
	diags.Append(callDiags...)
}

func (r *VDBSParameterGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VDBSParameterGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instID := state.InstanceID // preserve instance_id across Read

	found := r.readAndMerge(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	state.InstanceID = instID // restore instance_id

	if !instID.IsNull() && !instID.IsUnknown() && instID.ValueString() != "" {
		// Verify if it is still attached on the database instance
		instanceBody := map[string]interface{}{
			"id":          instID.ValueString(),
			"customer_id": r.customerID,
			"plan_type":   "dbs",
		}

		apiResp, instDiags := callAPI(ctx, r.client, pathDBInstanceDetail, instanceBody)
		if instDiags.HasError() {
			if apiResp != nil && isNotFoundMessage(apiResp.Message) {
				state.InstanceID = types.StringNull()
			}
		} else {
			var instData map[string]interface{}
			if err := json.Unmarshal(apiResp.Data, &instData); err == nil {
				currentPgName := asString(instData, "parameterGroupName")
				if currentPgName == "" {
					currentPgName = asString(instData, "parameter_group_name")
				}
				if currentPgName != "" && currentPgName != state.Name.ValueString() {
					state.InstanceID = types.StringNull()
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VDBSParameterGroupResource) readAndMerge(ctx context.Context, model *VDBSParameterGroupResourceModel, diags *diag.Diagnostics) bool {
	pgID := model.ID.ValueString()
	detailPath := fmt.Sprintf(pathParamGroupDetail, pgID)

	raw, err := r.client.DoMethod(ctx, http.MethodGet, detailPath, nil)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") || isNotFoundMessage(err.Error()) {
			return false
		}
		diags.AddError("Read metadata failed", err.Error())
		return false
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		diags.AddError("Parse Error", fmt.Sprintf("failed to parse parameter group detail: %s", err))
		return false
	}

	if idStr := asIDString(data, "id"); idStr != "" {
		model.ID = types.StringValue(idStr)
	}
	if name := asString(data, "name"); name != "" {
		model.Name = types.StringValue(name)
	}
	if desc := asString(data, "description"); desc != "" {
		model.Description = types.StringValue(desc)
	}
	if vpcIDResp := asIDString(data, "vpcId"); vpcIDResp != "" {
		model.VpcID = types.StringValue(vpcIDResp)
	}

	datastore := asString(data, "datastore")
	version := asString(data, "version")
	if datastore != "" && version != "" {
		model.Family = types.StringValue(datastore + version)
	}

	// 2. Fetch parameters list from separate endpoint
	paramPath := fmt.Sprintf(pathParamGroupParameters, pgID) + "?pageIndex=0&pageSize=1000"
	rawParams, err := r.client.DoMethod(ctx, http.MethodGet, paramPath, nil)
	if err != nil {
		diags.AddError("Read parameters failed", err.Error())
		return false
	}

	var paramData map[string]interface{}
	if err := json.Unmarshal(rawParams, &paramData); err != nil {
		diags.AddError("Parse Error", fmt.Sprintf("failed to parse parameters response: %s", err))
		return false
	}

	apiParamMap := make(map[string]string)
	if itemsRaw, ok := paramData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					name := asString(itemMap, "name")
					value := asString(itemMap, "value")
					if name != "" {
						apiParamMap[name] = value
					}
				}
			}
		}
	}

	// Read state parameters to preserve the exact set of parameters configured in HCL
	var stateParams []ParameterModel
	if !model.Parameters.IsNull() && !model.Parameters.IsUnknown() {
		_ = model.Parameters.ElementsAs(ctx, &stateParams, false)
	}

	updatedParams := make([]ParameterModel, 0, len(stateParams))
	for _, sp := range stateParams {
		name := sp.Name.ValueString()
		if val, exists := apiParamMap[name]; exists {
			updatedParams = append(updatedParams, ParameterModel{
				Name:  types.StringValue(name),
				Value: types.StringValue(val),
			})
		} else {
			updatedParams = append(updatedParams, sp)
		}
	}

	paramObjType := types.ObjectType{AttrTypes: parameterAttrTypes}
	listVal, listDiags := types.ListValueFrom(ctx, paramObjType, updatedParams)
	diags.Append(listDiags...)
	model.Parameters = listVal

	return true
}

func (r *VDBSParameterGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VDBSParameterGroupResourceModel
	var state VDBSParameterGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueString()
	if vpcID == "" {
		vpcID = state.VpcID.ValueString()
	}

	// Resolve host ID
	hostID, err := resolveHostID(ctx, r.client, r.customerID, vpcID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve region host ID", err.Error())
		return
	}

	pgID := state.ID.ValueString()

	// 1. If name or description changed, call rename API
	if plan.Name.ValueString() != state.Name.ValueString() || plan.Description.ValueString() != state.Description.ValueString() {
		body := map[string]interface{}{
			"name":        plan.Name.ValueString(),
			"description": plan.Description.ValueString(),
			"groupId":     pgID,
			"hostId":      hostID,
			"customerId":  r.customerID,
			"planType":    "dbs",
		}
		_, callDiags := callDBSAPI(ctx, r.client, http.MethodPut, pathParamGroupRename, body)
		resp.Diagnostics.Append(callDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// 2. If parameters changed, call updateParameters
	if !plan.Parameters.Equal(state.Parameters) {
		r.updateParameters(ctx, pgID, hostID, plan.Parameters, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// 3. If instance_id changed, attach to database instance
	if plan.InstanceID.ValueString() != state.InstanceID.ValueString() && plan.InstanceID.ValueString() != "" {
		serviceInit, err := resolveServiceInit(ctx, r.client, r.customerID, plan.InstanceID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to resolve database instance", err.Error())
			return
		}

		attachBody := map[string]interface{}{
			"serviceInitId":  serviceInit,
			"groupId":        pgID,
			"rebootRequired": true,
			"hostId":         hostID,
			"customerId":     r.customerID,
			"planType":       "dbs",
		}

		_, attachDiags := callDBSAPI(ctx, r.client, http.MethodPost, pathParamGroupAttach, attachBody)
		resp.Diagnostics.Append(attachDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	plan.ID = state.ID
	plan.VpcID = types.StringValue(vpcID)

	// Read back to get updated state.
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.InstanceID.IsUnknown() {
		plan.InstanceID = state.InstanceID
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VDBSParameterGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VDBSParameterGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	hostID, err := resolveHostID(ctx, r.client, r.customerID, vpcID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve region host ID", err.Error())
		return
	}

	pgID := state.ID.ValueString()
	deletePath := fmt.Sprintf(pathParamGroupDelete, pgID)

	body := map[string]interface{}{
		"hostId":     hostID,
		"customerId": r.customerID,
		"planType":   "dbs",
	}

	_, callDiags := callDBSAPI(ctx, r.client, http.MethodDelete, deletePath, body)
	if callDiags.HasError() {
		resp.Diagnostics.Append(callDiags...)
	}
}

func (r *VDBSParameterGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildParameterEntries converts the plan's parameter blocks to a JSON-serializable slice (AC: 2).
func buildParameterEntries(ctx context.Context, list types.List, diags *diag.Diagnostics) []parameterEntry {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var params []ParameterModel
	d := list.ElementsAs(ctx, &params, false)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}

	entries := make([]parameterEntry, 0, len(params))
	for _, p := range params {
		entries = append(entries, parameterEntry{
			Name:  p.Name.ValueString(),
			Value: p.Value.ValueString(),
		})
	}
	return entries
}

var familyRegex = regexp.MustCompile(`^([a-zA-Z]+)([0-9\.]+)$`)

func parseFamily(family string) (string, string, error) {
	matches := familyRegex.FindStringSubmatch(family)
	if len(matches) != 3 {
		return "", "", fmt.Errorf("invalid family format: %q (expected format like mysql8.0 or postgres14)", family)
	}
	return matches[1], matches[2], nil
}

func resolveHostID(ctx context.Context, c *client.Client, customerID string, vpcIDStr string) (int, error) {
	if vpcIDStr == "" {
		return 6, nil // Default host ID Mien Bac
	}

	targetID, err := strconv.Atoi(vpcIDStr)
	if err != nil {
		return 0, fmt.Errorf("vpc_id %q is not a valid integer: %w", vpcIDStr, err)
	}

	body := map[string]interface{}{
		"customerId": customerID,
		"planType":   "dbs",
	}

	raw, err := c.DoMethod(ctx, http.MethodPost, pathVpcList, body)
	if err != nil {
		return 0, fmt.Errorf("failed to query VPC list: %w", err)
	}

	var vpcs []map[string]interface{}
	if err := json.Unmarshal(raw, &vpcs); err != nil {
		return 0, fmt.Errorf("failed to parse VPC list response: %w", err)
	}

	for _, vpc := range vpcs {
		idVal, ok := vpc["id"]
		if !ok {
			continue
		}

		var idInt int
		switch v := idVal.(type) {
		case float64:
			idInt = int(v)
		case int:
			idInt = v
		case string:
			idInt, _ = strconv.Atoi(v)
		}

		if idInt == targetID {
			if cmpHostIDVal, ok := vpc["cmpHostId"]; ok {
				switch h := cmpHostIDVal.(type) {
				case float64:
					return int(h), nil
				case int:
					return h, nil
				case string:
					ret, _ := strconv.Atoi(h)
					return ret, nil
				}
			}
		}
	}

	return 6, nil // Default fallback
}

func resolveEngineVersion(ctx context.Context, c *client.Client, family string, hostID int, customerID string) (string, string, error) {
	datastore, version, err := parseFamily(family)
	if err != nil {
		return "", "", err
	}

	body := map[string]interface{}{
		"name":       datastore,
		"hostId":     hostID,
		"customerId": customerID,
		"planType":   "dbs",
	}

	raw, err := c.DoMethod(ctx, http.MethodPost, pathDatastoreVersionList, body)
	if err != nil {
		return "", "", fmt.Errorf("failed to query datastore versions: %w", err)
	}

	var versions []map[string]interface{}
	if err := json.Unmarshal(raw, &versions); err != nil {
		return "", "", fmt.Errorf("failed to parse datastore versions response: %w", err)
	}

	for _, v := range versions {
		nameVal := asString(v, "name")
		if nameVal == version {
			idVal := asIDString(v, "id")
			return datastore, idVal, nil
		}
	}

	return "", "", fmt.Errorf("version %q for engine %q not found in region host %d", version, datastore, hostID)
}

func callDBSAPI(ctx context.Context, c *client.Client, method string, path string, body interface{}) (*client.APIResponse, diag.Diagnostics) {
	var diags diag.Diagnostics
	raw, err := c.DoMethod(ctx, method, path, body)
	if err != nil {
		diags.AddError("API request failed", fmt.Sprintf("%s %s: %s", method, path, err.Error()))
		return nil, diags
	}

	if strings.TrimSpace(string(raw)) == "Success" {
		return &client.APIResponse{
			Code:    float64(0),
			Message: "Success",
		}, diags
	}

	resp, perr := client.ParseAPIResponse(raw)
	if perr != nil {
		diags.AddError("API response parse failed", fmt.Sprintf("%s %s: %s", method, path, perr.Error()))
		return nil, diags
	}

	if !resp.IsSuccess() {
		diags.AddError("Loi API", fmt.Sprintf("%s\n(%s %s code=%v)", resp.Message, method, path, resp.Code))
		return resp, diags
	}

	return resp, diags
}
