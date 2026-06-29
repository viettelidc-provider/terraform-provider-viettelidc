// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	sharedpd "terraform-provider-viettelidc/internal/providerdata"
	"terraform-provider-viettelidc/internal/service/vopc/client"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

// callAPI performs a POST to a DBS endpoint, parses the CSA envelope, and returns it.
// On transport error, parse error, or non-success CSA code an error diag is appended.
// The resp is returned even on CSA-level errors so callers can inspect resp.Message
// for not-found detection (idempotent Delete).
func callAPI(ctx context.Context, c *client.Client, path string, body map[string]interface{}) (*client.APIResponse, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Convert numeric-looking string IDs to integers for correct JSON encoding.
	// Note: customerId and vpcId for list endpoints must stay as strings!
	for _, k := range []string{"id", "vpc_id", "customer_id", "security_group_id"} {
		if v, ok := body[k]; ok {
			if s, isStr := v.(string); isStr {
				if n, err := strconv.Atoi(s); err == nil {
					body[k] = n
				}
			}
		}
	}

	if body != nil {
		if c.HostID != 0 {
			body["host_id"] = c.HostID
		}
	}

	raw, err := c.Do(ctx, path, body)

	if err != nil {
		if len(raw) > 0 {
			if parsed, perr := client.ParseAPIResponse(raw); perr == nil {
				diags.AddError("Loi API", fmt.Sprintf("%s\n(path=%s code=%v)", parsed.Message, path, parsed.Code))
				return parsed, diags
			}
		}
		diags.AddError("API request failed", fmt.Sprintf("path=%s: %s", path, err.Error()))
		return nil, diags
	}

	resp, perr := client.ParseAPIResponse(raw)
	if perr != nil {
		diags.AddError("API response parse failed", fmt.Sprintf("path=%s: %s", path, perr.Error()))
		return nil, diags
	}
	if !resp.IsSuccess() {
		diags.AddError("Loi API", fmt.Sprintf("%s\n(path=%s code=%v)", resp.Message, path, resp.Code))
		return resp, diags
	}
	return resp, diags
}

// providerDataFrom extracts *providerdata.ProviderData from the Configure() payload.
func providerDataFrom(providerData interface{}) (*providerdata.ProviderData, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	shared, ok := providerData.(*sharedpd.SharedProviderData)
	if !ok {
		diags.AddError(
			"Unexpected ProviderData Type",
			fmt.Sprintf("expected *sharedpd.SharedProviderData, got %T", providerData),
		)
		return nil, diags
	}
	return shared.IacData, diags
}

// resolveVpcID returns the effective VPC ID, preferring resource-level value
// over the provider default. Returns an error diag if both are empty.
func resolveVpcID(planVpcID, defaultVpcID string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if planVpcID != "" {
		return planVpcID, diags
	}
	if defaultVpcID != "" {
		return defaultVpcID, diags
	}
	diags.AddError(
		"vpc_id Required",
		"vpc_id must be set in the resource block or in the provider configuration (or VIETTELIDC_VPC_ID env var).",
	)
	return "", diags
}

// resolveVpcIDOptional returns the effective VPC ID if set, or empty string.
// Used by DBS datasources where vpc_id is optional and not required by the API.
func resolveVpcIDOptional(planVpcID, defaultVpcID string) string {
	if planVpcID != "" {
		return planVpcID
	}
	return defaultVpcID
}

// isNotFoundMessage returns true when CSA reports the resource does not exist.
func isNotFoundMessage(msg string) bool {
	if strings.Contains(strings.ToUpper(msg), "_NOT_FOUND") {
		return true
	}
	m := strings.ToLower(msg)
	return strings.Contains(m, "not found") ||
		strings.Contains(m, "not exist") ||
		strings.Contains(m, "no such") ||
		strings.Contains(m, "does not exist")
}

// asString safely extracts a string from a JSON-decoded map[string]interface{}.
func asString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		// Handle numeric types
		if f, ok := v.(float64); ok {
			return fmt.Sprintf("%.0f", f)
		}
		if i, ok := v.(int); ok {
			return fmt.Sprintf("%d", i)
		}
		if i64, ok := v.(int64); ok {
			return fmt.Sprintf("%d", i64)
		}
	}
	return ""
}

// filterListByID finds the first item in an items array where item["id"] matches targetID.
// Returns the item as map[string]interface{} or nil if not found.
func filterListByID(items interface{}, targetID string) map[string]interface{} {
	if items == nil {
		return nil
	}
	itemSlice, ok := items.([]interface{})
	if !ok {
		return nil
	}
	for _, item := range itemSlice {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if asIDString(itemMap, "id") == targetID {
				return itemMap
			}
		}
	}
	return nil
}

// filterListByName finds the first item in an items array where item["name"] matches targetName.
// Returns the item as map[string]interface{} or nil if not found.
func filterListByName(items interface{}, targetName string) map[string]interface{} {
	if items == nil {
		return nil
	}
	itemSlice, ok := items.([]interface{})
	if !ok {
		return nil
	}
	for _, item := range itemSlice {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if asString(itemMap, "name") == targetName {
				return itemMap
			}
		}
	}
	return nil
}

// asIDString extracts an ID field that may be a JSON string or a JSON number.
func asIDString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == 0 {
			return ""
		}
		return strconv.FormatInt(int64(val), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// asInt64 extracts a float64/string field as int64.
func asInt64(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// listFromJSONArray extracts a JSON array field from a decoded map and converts
// it to a types.List of strings. Returns an empty list when the key is absent.
func listFromJSONArray(ctx context.Context, m map[string]interface{}, key string) (types.List, diag.Diagnostics) {
	var strs []string
	if raw, ok := m[key]; ok {
		if arr, ok := raw.([]interface{}); ok {
			for _, item := range arr {
				strs = append(strs, fmt.Sprintf("%v", item))
			}
		}
	}
	return types.ListValueFrom(ctx, types.StringType, strs)
}

// resolveServiceInit resolves the instanceIDInput (which can be a name, ID, or UUID) to its serviceInit ID.
func resolveServiceInit(ctx context.Context, c *client.Client, customerID, instanceIDInput string) (int64, error) {
	isUUID := len(instanceIDInput) == 36 && strings.Contains(instanceIDInput, "-")

	body := map[string]interface{}{
		
		"customer_id": customerID,
		"plan_type":   "dbs",
	}

	apiResp, callDiags := callAPI(ctx, c, pathDBInstanceList, body)
	if callDiags.HasError() {
		return 0, fmt.Errorf("failed to list database instances: %v", callDiags)
	}

	if apiResp == nil || apiResp.Data == nil {
		return 0, fmt.Errorf("no database instances data found")
	}

	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		return 0, err
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		return 0, err
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

	for _, inst := range instances {
		idStr := asString(inst, "id")
		nameStr := asString(inst, "name")
		vttIDStr := asString(inst, "vttDbaasInstanceId")
		serviceInit := asInt64(inst, "serviceInit")

		if isUUID {
			uuid, err := fetchInstanceUUID(ctx, c, customerID, serviceInit)
			if err == nil && uuid == instanceIDInput {
				return serviceInit, nil
			}
		} else {
			if idStr == instanceIDInput || nameStr == instanceIDInput || vttIDStr == instanceIDInput {
				if serviceInit != 0 {
					return serviceInit, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("database instance %q not found or does not have serviceInit", instanceIDInput)
}

func fetchInstanceUUID(ctx context.Context, c *client.Client, customerID string, serviceInit int64) (string, error) {
	schemaBody := map[string]interface{}{
		"page":        0,
		"pageSize":    100,
		"serviceInit": serviceInit,
		"hostId":      6,
		"customerId":  customerID,
		"planType":    "dbs",
	}

	schemaResp, callDiags := callAPI(ctx, c, pathDBSchemaList, schemaBody)
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

