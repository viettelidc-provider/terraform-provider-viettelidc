// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*VDBSDatabaseUserResource)(nil)
	_ resource.ResourceWithConfigure   = (*VDBSDatabaseUserResource)(nil)
	_ resource.ResourceWithImportState = (*VDBSDatabaseUserResource)(nil)
)

func NewVDBSDatabaseUserResource() resource.Resource {
	return &VDBSDatabaseUserResource{}
}

type VDBSDatabaseUserResource struct {
	client     *client.Client
	customerID string
}

type VDBSDatabaseUserResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Password   types.String `tfsdk:"password"`
	InstanceID types.String `tfsdk:"instance_id"`
	Host       types.String `tfsdk:"host"`
	Schemas    types.Set    `tfsdk:"schemas"`
}

func (r *VDBSDatabaseUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_database_user"
}

func (r *VDBSDatabaseUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC VDBS Database User — configures database users and schema permissions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Database user UUID assigned by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Database username.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Database user password.",
			},
			"instance_id": schema.StringAttribute{
				Required:    true,
				Description: "Database instance ID (can be numeric ID, UUID, or name).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"host": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("%"),
				Description: "Host mask. Defaults to '%'.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schemas": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Set of database schema names granted to this user.",
			},
		},
	}
}

func (r *VDBSDatabaseUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
}

func (r *VDBSDatabaseUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VDBSDatabaseUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceInit, _, err := r.resolveServiceInitAndUUID(ctx, plan.InstanceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve database instance", err.Error())
		return
	}

	host := plan.Host.ValueString()
	hostType := "custom"
	if host == "%" {
		hostType = "any"
	}

	body := map[string]interface{}{
		"name":                plan.Name.ValueString(),
		"password":            plan.Password.ValueString(),
		"host_type":           hostType,
		"listSchemaGrant":     []interface{}{map[string]interface{}{}},
		"host":                host,
		"serviceInitExtendId": 0,
		"serviceInit":         serviceInit,
		"hostId":              6,
		"customerId":          r.customerID,
		"planType":            "dbs",
	}

	_, callDiags := callAPI(ctx, r.client, pathDBUserCreate, body)
	resp.Diagnostics.Append(callDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Match user to get UUID from user list
	listBody := map[string]interface{}{
		"pageIndex":           0,
		"pageSize":            100,
		"filters":             []interface{}{},
		"serviceInitExtendId": 0,
		"serviceInit":         serviceInit,
		"hostId":              6,
		"customerId":          r.customerID,
		"planType":            "dbs",
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathDBUserList, listBody)
	resp.Diagnostics.Append(callDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		resp.Diagnostics.AddError("decode error", err.Error())
		return
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		resp.Diagnostics.AddError("decode error", err.Error())
		return
	}

	var users []map[string]interface{}
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					users = append(users, itemMap)
				}
			}
		}
	}

	var foundUser map[string]interface{}
	for _, u := range users {
		if asString(u, "name") == plan.Name.ValueString() && asString(u, "host") == host {
			foundUser = u
			break
		}
	}

	if foundUser == nil {
		resp.Diagnostics.AddError("User not found after creation", "Could not locate the newly created user in the database instance user list.")
		return
	}

	foundID := asString(foundUser, "id")
	plan.ID = types.StringValue(foundID)

	// Fetch actual schema grants via list_revoke API
	revokeBody := map[string]interface{}{
		"id":         foundID,
		"hostId":     6,
		"customerId": r.customerID,
		"planType":   "dbs",
	}
	apiRevokeResp, revokeDiags := callAPI(ctx, r.client, "/dbs/api/v1/schema/list_revoke", revokeBody)
	if revokeDiags.HasError() {
		resp.Diagnostics.Append(revokeDiags...)
		return
	}

	var defaultGranted []string
	if apiRevokeResp != nil && apiRevokeResp.Data != nil {
		rawRevoke, err := json.Marshal(apiRevokeResp.Data)
		if err == nil {
			var revokeItems []map[string]interface{}
			if err := json.Unmarshal(rawRevoke, &revokeItems); err == nil {
				for _, item := range revokeItems {
					name := asString(item, "name")
					if name != "" {
						defaultGranted = append(defaultGranted, name)
					}
				}
			}
		}
	}

	// Calculate schemas to grant
	var desired []string
	if !plan.Schemas.IsNull() && !plan.Schemas.IsUnknown() {
		resp.Diagnostics.Append(plan.Schemas.ElementsAs(ctx, &desired, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	defaultGrantedMap := make(map[string]bool)
	for _, s := range defaultGranted {
		defaultGrantedMap[s] = true
	}

	var toGrant []string
	for _, s := range desired {
		if !defaultGrantedMap[s] {
			toGrant = append(toGrant, s)
		}
	}

	if len(toGrant) > 0 {
		if err := r.modifySchemaGrants(ctx, foundID, serviceInit, toGrant, nil); err != nil {
			resp.Diagnostics.AddError("Failed to grant schemas", err.Error())
			return
		}
		if err := r.pollUntilUserSchemasUpdated(ctx, foundID, desired, 1*time.Minute); err != nil {
			resp.Diagnostics.AddWarning("Schema Update Polling Timeout", err.Error())
		}
	}

	// Read and merge properties
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VDBSDatabaseUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VDBSDatabaseUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found := r.readAndMerge(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VDBSDatabaseUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VDBSDatabaseUserResourceModel
	var state VDBSDatabaseUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceInit, _, err := r.resolveServiceInitAndUUID(ctx, plan.InstanceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve database instance", err.Error())
		return
	}

	userID := state.ID.ValueString()

	// Update Password if changed
	if plan.Password.ValueString() != state.Password.ValueString() {
		body := map[string]interface{}{
			"id":          userID,
			"newPassword": plan.Password.ValueString(),
			"confirm":     plan.Password.ValueString(),
			"hostId":      6,
			"customerId":  r.customerID,
			"planType":    "dbs",
		}
		_, callDiags := callAPI(ctx, r.client, pathDBUserUpdate, body)
		resp.Diagnostics.Append(callDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Update schema grants if changed
	var planSchemas []string
	var stateSchemas []string
	if !plan.Schemas.IsNull() && !plan.Schemas.IsUnknown() {
		resp.Diagnostics.Append(plan.Schemas.ElementsAs(ctx, &planSchemas, false)...)
	}
	if !state.Schemas.IsNull() && !state.Schemas.IsUnknown() {
		resp.Diagnostics.Append(state.Schemas.ElementsAs(ctx, &stateSchemas, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Compare schemas to find additions and removals
	planMap := make(map[string]bool)
	for _, s := range planSchemas {
		planMap[s] = true
	}
	stateMap := make(map[string]bool)
	for _, s := range stateSchemas {
		stateMap[s] = true
	}

	var toGrant []string
	for _, s := range planSchemas {
		if !stateMap[s] {
			toGrant = append(toGrant, s)
		}
	}

	var toRevoke []string
	for _, s := range stateSchemas {
		if !planMap[s] {
			toRevoke = append(toRevoke, s)
		}
	}

	if len(toGrant) > 0 || len(toRevoke) > 0 {
		if err := r.modifySchemaGrants(ctx, userID, serviceInit, toGrant, toRevoke); err != nil {
			resp.Diagnostics.AddError("Failed to update schema grants", err.Error())
			return
		}
		if err := r.pollUntilUserSchemasUpdated(ctx, userID, planSchemas, 1*time.Minute); err != nil {
			resp.Diagnostics.AddWarning("Schema Update Polling Timeout", err.Error())
		}
	}

	plan.ID = state.ID
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VDBSDatabaseUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VDBSDatabaseUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceInit, _, err := r.resolveServiceInitAndUUID(ctx, state.InstanceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve database instance", err.Error())
		return
	}

	// Fetch full user object from list to send to delete API
	listBody := map[string]interface{}{
		"pageIndex":           0,
		"pageSize":            100,
		"filters":             []interface{}{},
		"serviceInitExtendId": 0,
		"serviceInit":         serviceInit,
		"hostId":              6,
		"customerId":          r.customerID,
		"planType":            "dbs",
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathDBUserList, listBody)
	if callDiags.HasError() {
		resp.Diagnostics.Append(callDiags...)
		return
	}

	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		resp.Diagnostics.AddError("decode error", err.Error())
		return
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		resp.Diagnostics.AddError("decode error", err.Error())
		return
	}

	var users []map[string]interface{}
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					users = append(users, itemMap)
				}
			}
		}
	}

	var targetUser map[string]interface{}
	for _, u := range users {
		if asString(u, "id") == state.ID.ValueString() {
			targetUser = u
			break
		}
	}

	if targetUser == nil {
		// Already deleted
		return
	}

	// Inject standard API routing parameters
	targetUser["hostId"] = 6
	targetUser["customerId"] = r.customerID
	targetUser["planType"] = "dbs"

	_, callDiags = callAPI(ctx, r.client, pathDBUserDelete, targetUser)
	resp.Diagnostics.Append(callDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Poll until the user is actually deleted
	if err := r.pollUntilUserDeleted(ctx, serviceInit, state.ID.ValueString(), 15*time.Minute); err != nil {
		resp.Diagnostics.AddWarning("Delete Polling Timeout", err.Error())
	}
}

func (r *VDBSDatabaseUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by format: "instance_id/user_id"
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid Import ID", "Import database user expects format: <instance_id>/<user_id>")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func (r *VDBSDatabaseUserResource) readAndMerge(ctx context.Context, model *VDBSDatabaseUserResourceModel, diags *diag.Diagnostics) bool {
	serviceInit, _, err := r.resolveServiceInitAndUUID(ctx, model.InstanceID.ValueString())
	if err != nil {
		diags.AddError("Failed to resolve database instance", err.Error())
		return false
	}

	listBody := map[string]interface{}{
		"pageIndex":           0,
		"pageSize":            100,
		"filters":             []interface{}{},
		"serviceInitExtendId": 0,
		"serviceInit":         serviceInit,
		"hostId":              6,
		"customerId":          r.customerID,
		"planType":            "dbs",
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathDBUserList, listBody)
	if callDiags.HasError() {
		diags.Append(callDiags...)
		return false
	}

	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		diags.AddError("decode error", err.Error())
		return false
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		diags.AddError("decode error", err.Error())
		return false
	}

	var users []map[string]interface{}
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					users = append(users, itemMap)
				}
			}
		}
	}

	targetID := model.ID.ValueString()
	var found map[string]interface{}
	for _, u := range users {
		if asString(u, "id") == targetID {
			found = u
			break
		}
	}

	if found == nil {
		return false
	}

	model.Name = types.StringValue(asString(found, "name"))
	model.Host = types.StringValue(asString(found, "host"))

	// Fetch actual schema grants via list_revoke API
	revokeBody := map[string]interface{}{
		"id":         targetID,
		"hostId":     6,
		"customerId": r.customerID,
		"planType":   "dbs",
	}
	apiRevokeResp, revokeDiags := callAPI(ctx, r.client, "/dbs/api/v1/schema/list_revoke", revokeBody)
	if revokeDiags.HasError() {
		diags.Append(revokeDiags...)
		return false
	}

	var schemaNames []string
	if apiRevokeResp != nil && apiRevokeResp.Data != nil {
		rawRevoke, err := json.Marshal(apiRevokeResp.Data)
		if err == nil {
			var revokeItems []map[string]interface{}
			if err := json.Unmarshal(rawRevoke, &revokeItems); err == nil {
				for _, item := range revokeItems {
					name := asString(item, "name")
					if name != "" {
						schemaNames = append(schemaNames, name)
					}
				}
			}
		}
	}

	if len(schemaNames) > 0 {
		schemaList, schemaDiags := types.SetValueFrom(ctx, types.StringType, schemaNames)
		diags.Append(schemaDiags...)
		if !diags.HasError() {
			model.Schemas = schemaList
		}
	} else if model.Schemas.IsNull() || model.Schemas.IsUnknown() {
		schemaList, _ := types.SetValueFrom(ctx, types.StringType, []string{})
		model.Schemas = schemaList
	}

	return true
}

func (r *VDBSDatabaseUserResource) fetchInstanceUUID(ctx context.Context, serviceInit int64) (string, error) {
	schemaBody := map[string]interface{}{
		"page":        0,
		"pageSize":    100,
		"serviceInit": serviceInit,
		"hostId":      6,
		"customerId":  r.customerID,
		"planType":    "dbs",
	}

	schemaResp, callDiags := callAPI(ctx, r.client, pathDBSchemaList, schemaBody)
	if callDiags.HasError() {
		return "", fmt.Errorf("failed to fetch schema list: %v", callDiags)
	}

	if schemaResp == nil || schemaResp.Data == nil {
		return "", fmt.Errorf("no schema list data found")
	}

	rawSchemas, err := json.Marshal(schemaResp.Data)
	if err != nil {
		return "", err
	}

	var schemaListData map[string]interface{}
	if err := json.Unmarshal(rawSchemas, &schemaListData); err != nil {
		return "", err
	}

	var schemas []map[string]interface{}
	if itemsRaw, ok := schemaListData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					schemas = append(schemas, itemMap)
				}
			}
		}
	}

	if len(schemas) == 0 {
		return "", fmt.Errorf("no schemas found for database instance (serviceInit %d)", serviceInit)
	}

	uuid := asString(schemas[0], "instanceId")
	if uuid == "" {
		return "", fmt.Errorf("instanceId is empty in schema list")
	}

	return uuid, nil
}

func (r *VDBSDatabaseUserResource) resolveServiceInitAndUUID(ctx context.Context, instanceIDInput string) (int64, string, error) {
	body := map[string]interface{}{
		"customer_id": r.customerID,
		"plan_type":   "dbs",
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathDBInstanceList, body)
	if callDiags.HasError() {
		return 0, "", fmt.Errorf("failed to list database instances: %v", callDiags)
	}

	if apiResp == nil || apiResp.Data == nil {
		return 0, "", fmt.Errorf("no database instances data found")
	}

	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		return 0, "", err
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		return 0, "", err
	}

	var instances []map[string]interface{}
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					instances = append(instances, itemMap)
				}
			}
		}
	}

	var foundInstance map[string]interface{}
	var instanceUUID string

	isUUID := len(instanceIDInput) == 36 && strings.Contains(instanceIDInput, "-")

	for _, inst := range instances {
		idStr := asString(inst, "id")
		nameStr := asString(inst, "name")
		vttIDStr := asString(inst, "vttDbaasInstanceId")
		serviceInit := asInt64(inst, "serviceInit")

		if isUUID {
			uuid, err := r.fetchInstanceUUID(ctx, serviceInit)
			if err == nil && uuid == instanceIDInput {
				foundInstance = inst
				instanceUUID = uuid
				break
			}
		} else {
			if idStr == instanceIDInput || nameStr == instanceIDInput || vttIDStr == instanceIDInput {
				foundInstance = inst
				break
			}
		}
	}

	if foundInstance == nil {
		return 0, "", fmt.Errorf("database instance %q not found", instanceIDInput)
	}

	serviceInit := asInt64(foundInstance, "serviceInit")
	if serviceInit == 0 {
		return 0, "", fmt.Errorf("database instance %q does not have serviceInit populated", instanceIDInput)
	}

	if instanceUUID == "" {
		uuid, err := r.fetchInstanceUUID(ctx, serviceInit)
		if err != nil {
			return 0, "", err
		}
		instanceUUID = uuid
	}

	return serviceInit, instanceUUID, nil
}

func (r *VDBSDatabaseUserResource) customerIdString(serviceInit int64) string {
	// If customerID is empty, try to get it from context/defaults if possible
	return r.customerID
}

func (r *VDBSDatabaseUserResource) fetchSchemasMap(ctx context.Context, serviceInit int64) (map[string]string, error) {
	schemaBody := map[string]interface{}{
		"page":        0,
		"pageSize":    100,
		"serviceInit": serviceInit,
		"hostId":      6,
		"customerId":  r.customerID,
		"planType":    "dbs",
	}

	schemaResp, callDiags := callAPI(ctx, r.client, pathDBSchemaList, schemaBody)
	if callDiags.HasError() {
		return nil, fmt.Errorf("failed to fetch schema list: %v", callDiags)
	}

	if schemaResp == nil || schemaResp.Data == nil {
		return nil, fmt.Errorf("no schema list data found")
	}

	rawSchemas, err := json.Marshal(schemaResp.Data)
	if err != nil {
		return nil, err
	}

	var schemaListData map[string]interface{}
	if err := json.Unmarshal(rawSchemas, &schemaListData); err != nil {
		return nil, err
	}

	var items []interface{}
	if itemsRaw, ok := schemaListData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			items = itemsArr
		}
	}

	res := make(map[string]string)
	for _, item := range items {
		if itemMap, ok := item.(map[string]interface{}); ok {
			name := asString(itemMap, "name")
			id := asString(itemMap, "id")
			if name != "" && id != "" {
				res[name] = id
			}
		}
	}

	return res, nil
}

func (r *VDBSDatabaseUserResource) modifySchemaGrants(ctx context.Context, userID string, serviceInit int64, toGrant, toRevoke []string) error {
	if len(toGrant) == 0 && len(toRevoke) == 0 {
		return nil
	}

	schemasMap, err := r.fetchSchemasMap(ctx, serviceInit)
	if err != nil {
		return err
	}

	if len(toGrant) > 0 {
		var listGrant []map[string]string
		for _, name := range toGrant {
			id, ok := schemasMap[name]
			if !ok {
				return fmt.Errorf("schema %q not found in database instance", name)
			}
			listGrant = append(listGrant, map[string]string{
				"id":   id,
				"name": name,
			})
		}
		body := map[string]interface{}{
			"id":              userID,
			"listSchemaGrant": listGrant,
			"hostId":          6,
			"customerId":      r.customerID,
			"planType":        "dbs",
		}
		_, callDiags := callAPI(ctx, r.client, pathDBUserGrant, body)
		if callDiags.HasError() {
			return fmt.Errorf("failed to grant schemas: %v", callDiags)
		}
	}

	if len(toRevoke) > 0 {
		var listRevoke []map[string]string
		for _, name := range toRevoke {
			id, ok := schemasMap[name]
			if !ok {
				return fmt.Errorf("schema %q not found in database instance", name)
			}
			listRevoke = append(listRevoke, map[string]string{
				"id":   id,
				"name": name,
			})
		}
		body := map[string]interface{}{
			"id":               userID,
			"listSchemaRevoke": listRevoke,
			"hostId":           6,
			"customerId":       r.customerID,
			"planType":         "dbs",
		}
		_, callDiags := callAPI(ctx, r.client, pathDBUserRevoke, body)
		if callDiags.HasError() {
			return fmt.Errorf("failed to revoke schemas: %v", callDiags)
		}
	}
	return nil
}

func (r *VDBSDatabaseUserResource) pollUntilUserDeleted(ctx context.Context, serviceInit int64, userID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		listBody := map[string]interface{}{
			"pageIndex":           0,
			"pageSize":            100,
			"filters":             []interface{}{},
			"serviceInitExtendId": 0,
			"serviceInit":         serviceInit,
			"hostId":              6,
			"customerId":          r.customerID,
			"planType":            "dbs",
		}

		apiResp, callDiags := callAPI(ctx, r.client, pathDBUserList, listBody)
		if !callDiags.HasError() && apiResp != nil && apiResp.Data != nil {
			raw, err := json.Marshal(apiResp.Data)
			if err == nil {
				var listData map[string]interface{}
				if err := json.Unmarshal(raw, &listData); err == nil {
					found := false
					if itemsRaw, ok := listData["items"]; ok {
						if itemsArr, ok := itemsRaw.([]interface{}); ok {
							for _, item := range itemsArr {
								if itemMap, ok := item.(map[string]interface{}); ok {
									if asString(itemMap, "id") == userID {
										found = true
										break
									}
								}
							}
						}
					}
					if !found {
						// User not found -> successfully deleted
						return nil
					}
				}
			}
		}

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("timeout waiting for database user %s to be deleted", userID)
}

func (r *VDBSDatabaseUserResource) pollUntilUserSchemasUpdated(ctx context.Context, userID string, desired []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	desiredMap := make(map[string]bool)
	for _, s := range desired {
		desiredMap[s] = true
	}

	for time.Now().Before(deadline) {
		revokeBody := map[string]interface{}{
			"id":         userID,
			"hostId":     6,
			"customerId": r.customerID,
			"planType":   "dbs",
		}

		apiRevokeResp, callDiags := callAPI(ctx, r.client, "/dbs/api/v1/schema/list_revoke", revokeBody)
		if !callDiags.HasError() && apiRevokeResp != nil && apiRevokeResp.Data != nil {
			rawRevoke, err := json.Marshal(apiRevokeResp.Data)
			if err == nil {
				var revokeItems []map[string]interface{}
				if err := json.Unmarshal(rawRevoke, &revokeItems); err == nil {
					actualGranted := make(map[string]bool)
					for _, item := range revokeItems {
						name := asString(item, "name")
						if name != "" {
							actualGranted[name] = true
						}
					}

					allPresent := true
					for s := range desiredMap {
						if !actualGranted[s] {
							allPresent = false
							break
						}
					}

					if allPresent {
						return nil
					}
				}
			}
		}

		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("timeout waiting for database user %s schemas to be updated", userID)
}
