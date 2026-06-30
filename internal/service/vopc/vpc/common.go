// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package vpc implements ViettelIDC IaC Phase 4 resources and data sources
// for autoscaling: Launch Template and Autoscale Group.
//
// All resources call API via API Gateway on /terraform/v1/vpc/... paths.
// The API Gateway rewrites these to /csa/api/v1/vpc/... and
// renames snake_case body fields to camelCase.
package vpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	sharedpd "terraform-provider-viettelidc/internal/providerdata"
	"terraform-provider-viettelidc/internal/service/vopc/client"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

// callAPI performs a POST to a routed CSA endpoint, parses the standard
// CSA envelope, and returns it. On transport error, parse error, or non-success
// CSA code, an error diag is appended.
//
// The resp value is returned even on CSA-level errors so callers can inspect
// resp.Message to detect "not found" for idempotent Delete.
func callAPI(ctx context.Context, c *client.Client, path string, body map[string]interface{}) (*client.APIResponse, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Convert numeric-looking string IDs to integers so they are forwarded correctly.
	for _, k := range []string{"id", "vpc_id", "customer_id"} {
		if v, ok := body[k]; ok {
			if s, isStr := v.(string); isStr {
				if n, err := strconv.Atoi(s); err == nil {
					body[k] = n
				}
			}
		}
	}

	raw, err := c.Do(ctx, path, body)
	if err != nil {
		if len(raw) > 0 {
			if parsed, perr := client.ParseAPIResponse(raw); perr == nil {
				diags.AddError("Lỗi API", fmt.Sprintf("%s\n(path=%s code=%v)", parsed.Message, path, parsed.Code))
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
		diags.AddError("Lỗi API", fmt.Sprintf("%s\n(path=%s code=%v)", resp.Message, path, resp.Code))
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
// over the provider default.
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

// isNotFoundMessage returns true when CSA reports a resource does not exist.
func isNotFoundMessage(msg string) bool {
	// Fast path: raw API error codes end with _NOT_FOUND (e.g. ERROR_VM_NOT_FOUND).
	if strings.Contains(strings.ToUpper(msg), "_NOT_FOUND") {
		return true
	}
	// Fallback: translated / human-readable messages.
	m := strings.ToLower(msg)
	return strings.Contains(m, "not found") ||
		strings.Contains(m, "not exist") ||
		strings.Contains(m, "no such") ||
		strings.Contains(m, "does not exist")
}

// isInUseMessage returns true when CSA reports a resource is still in use.
func isInUseMessage(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "in use") ||
		strings.Contains(m, "associated") ||
		strings.Contains(m, "being used") ||
		strings.Contains(m, "still used")
}

// asString safely extracts a string from a JSON-decoded map.
func asString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// asIDString extracts an ID field that may be a JSON string or number.
func asIDString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		switch id := v.(type) {
		case string:
			return id
		case float64:
			if id > 0 {
				return fmt.Sprintf("%d", int(id))
			}
		}
	}
	return ""
}

// asInt64 safely extracts an int64 from a JSON-decoded map.
func asInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int64:
			return n
		case int:
			return int64(n)
		}
	}
	return 0
}

// asBool safely extracts a bool from a JSON-decoded map.
func asBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// decodeData unmarshals the API response data field into a map.
func decodeData(resp *client.APIResponse) (map[string]interface{}, error) {
	if resp == nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty response data")
	}
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to decode response data: %w", err)
	}
	return data, nil
}
