package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-viettelidc/internal/service/iac/client"
	"terraform-provider-viettelidc/internal/service/iac/providerdata"
	sharedpd "terraform-provider-viettelidc/internal/providerdata"
)

// callAPI performs a POST to a routed CSA endpoint, parses the standard
// CSA envelope, and returns it. On transport error, parse error, or non-success
// CSA code, an error diag is appended and the second return slot is non-empty.
//
// The resp value is returned even on CSA-level errors (so callers can inspect
// resp.Message to detect "not found" cases for idempotent Delete).
func callAPI(ctx context.Context, c *client.Client, path string, body map[string]interface{}) (*client.APIResponse, diag.Diagnostics) {
	var diags diag.Diagnostics

	// The API Gateway renames vpc_id→vpcId / customer_id→customerId
	// but preserves the JSON type. Convert here so they are forwarded as integers.
	for _, k := range []string{"id", "vpc_id", "customer_id", "vtt_subnet_id", "vtt_internet_gateway_id", "vtt_nat_id"} {
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
		// raw may still hold the response body for HTTP-level errors; try to
		// parse so callers can detect "not found" via message.
		if len(raw) > 0 {
			if parsed, perr := client.ParseAPIResponse(raw); perr == nil {
				diags.AddError("Lỗi API", fmt.Sprintf("%s\n(path=%s code=%v)", translateCSAError(parsed.Message), path, parsed.Code))
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
		diags.AddError("Lỗi API",
			fmt.Sprintf("%s\n(path=%s code=%v)", translateCSAError(resp.Message), path, resp.Code))
		return resp, diags
	}
	return resp, diags
}

// translateCSAError converts a API error code (ERROR_XXX) into a readable
// English message. Unrecognised codes are returned as-is.
func translateCSAError(msg string) string {
	translations := map[string]string{
		// General
		"ERROR_COMMON_0001":                      "Invalid request — please check all required parameters.",
		"ERROR_BODY_REQUEST_IS_NOT_VALID":         "Request body is invalid — please verify all required fields.",
		"ERROR_UNAUTHORIZED":                      "Unauthorized — session has expired or the token is invalid.",
		"ERROR_FORBIDDEN":                         "Forbidden — your account does not have permission to perform this action.",
		"ERROR_NOT_FOUND":                         "Resource not found or has already been deleted.",
		"ERROR_INTERNAL_SERVER":                   "Internal server error — please try again later.",
		// Subnet
		"ERROR_SUBNET_NAME_IS_EXISTED":            "A subnet with that name already exists in this VPC — choose a different name.",
		"ERROR_SUBNET_NOT_FOUND":                  "Subnet not found or has already been deleted.",
		"ERROR_SUBNET_CIDR_IS_INVALID":            "Subnet CIDR block is invalid.",
		"ERROR_SUBNET_CIDR_IS_OVERLAPPING":        "Subnet CIDR overlaps with another subnet in this VPC.",
		// VPC
		"ERROR_VPC_NOT_FOUND":                     "VPC not found or has already been deleted.",
		"ERROR_VPC_NAME_IS_EXISTED":               "A VPC with that name already exists — choose a different name.",
		"ERROR_VPC_CIDR_IS_INVALID":               "VPC CIDR block is invalid.",
		// Security Group
		"ERROR_SECURITY_GROUP_NAME_IS_EXISTED":    "A security group with that name already exists in this VPC — choose a different name.",
		"ERROR_SECURITY_GROUP_NOT_FOUND":          "Security group not found or has already been deleted.",
		"ERROR_SECURITY_GROUP_RULE_NOT_FOUND":     "Security group rule not found.",
		"ERROR_SECURITY_GROUP_IN_USE":             "Security group is still in use and cannot be deleted.",
		// Key Pair
		"KEY_PAIR_EXISTED":                        "A key pair with that name already exists — choose a different name or delete the existing one first.",
		"KEY_PAIR_NOT_FOUND":                      "Key pair not found or has already been deleted.",
		// NIC
		"ERROR_NIC_NOT_FOUND":                     "Network interface not found or has already been deleted.",
		"ERROR_NIC_NAME_IS_EXISTED":               "A network interface with that name already exists — choose a different name.",
		"ERROR_NIC_IS_ATTACHED":                   "Network interface is currently attached to a VM — detach it first.",
		// VM / Instance
		"ERROR_VM_NOT_FOUND":                      "Instance not found or has already been deleted.",
		"ERROR_VM_NAME_IS_EXISTED":                "An instance with that name already exists — choose a different name.",
		"ERROR_VM_TEMPLATE_NOT_FOUND":             "VM template not found — check that template_id is correct.",
		"ERROR_VM_FLAVOR_NOT_FOUND":               "VM flavor (size) not found.",
		"ERROR_VM_IS_RUNNING":                     "Instance is currently running — stop it before performing this action.",
		// Volume
		"ERROR_VOLUME_NOT_FOUND":                  "Volume not found or has already been deleted.",
		"ERROR_VOLUME_NAME_IS_EXISTED":            "A volume with that name already exists — choose a different name.",
		"ERROR_VOLUME_IS_ATTACHED":                "Volume is currently attached to a VM — detach it first.",
		"ERROR_VOLUME_IN_USE":                     "Volume is in use and cannot be deleted.",
		// Floating IP
		"ERROR_FLOATING_IP_NOT_FOUND":             "Floating IP not found or has already been deleted.",
		"ERROR_FLOATING_IP_IS_ASSOCIATED":         "Floating IP is already associated — disassociate it first.",
		// Route Table
		"ERROR_ROUTE_TABLE_NOT_FOUND":             "Route table not found or has already been deleted.",
		// Load Balancer
		"ERROR_LOAD_BALANCER_NOT_FOUND":           "Load balancer not found or has already been deleted.",
		"ERROR_LOAD_BALANCER_NAME_IS_EXISTED":     "A load balancer with that name already exists — choose a different name.",
	}
	if readable, ok := translations[msg]; ok {
		return fmt.Sprintf("%s (code: %s)", readable, msg)
	}
	return msg
}


// "resource does not exist". Used by Delete idempotency and Read drift
// detection.
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

// isNotAttachedMessage returns true when CSA reports a NIC was not attached
// at detach time (already-detached scenario, treat as success).
func isNotAttachedMessage(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "not attached") ||
		strings.Contains(m, "no attachment") ||
		strings.Contains(m, "already detached")
}

// providerDataFrom extracts the *providerdata.ProviderData from the opaque
// Configure() request payload. Returns (nil, nil) when providerData is nil
// (which is normal during early-phase Configure calls — caller must guard).
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

// resolveVpcID returns the effective VPC ID, preferring the resource-level
// value over the provider default. Returns an error diag if both are empty.
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
		"vpc_id must be set in the resource/data source block or in the provider configuration (or VIETTELIDC_VPC_ID env var).",
	)
	return "", diags
}

// asString safely extracts a string from a JSON-decoded map[string]interface{}.
// Returns "" when the key is missing or the value is not a string.
func asString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// asIDString extracts an ID field that may be a JSON string or a JSON number.
// Returns "" when the key is missing or the value is zero/empty.
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

// pollUntilReady polls a API detail endpoint every 3 seconds until the
// response contains status:"success", the timeout is reached, or ctx is
// cancelled. Non-fatal polling errors are silently retried.
func pollUntilReady(ctx context.Context, c *client.Client, path string, body map[string]interface{}, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, _ := callAPI(ctx, c, path, body) // ignore transient errors
		if resp != nil {
			var data map[string]interface{}
			if err := json.Unmarshal(resp.Data, &data); err == nil {
				status := asString(data, "status")
				if status == "success" || status == "ACTIVE" || status == "active" || status == "AVAILABLE" || status == "available" || status == "IN-USE" {
					return nil
				}
				if status == "error" || status == "failed" || status == "ERROR" || status == "FAILED" {
					return fmt.Errorf("resource entered error state: status=%s", status)
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for resource to become ready (timeout=%s)", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// asBool safely extracts a bool from a JSON-decoded map[string]interface{}.
func asBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

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

// callKMS performs a REST call to the key-manager service.
// Unlike callAPI, key-manager responses are NOT CSA envelopes — they are
// either empty (on success) or raw JSON. Returns raw bytes.
func callKMS(ctx context.Context, c *client.Client, method, path string, body interface{}) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	raw, err := c.DoMethod(ctx, method, path, body)
	if err != nil {
		diags.AddError("Key-manager request failed",
			fmt.Sprintf("method=%s path=%s: %s", method, path, err.Error()))
		return nil, diags
	}
	return raw, diags
}

// pollForStatus polls a API detail endpoint every 10 seconds until the
// resource status matches one of the targetStatuses, an error state is detected
// (error/failed), the timeout elapses, or ctx is cancelled.
// The statusKey parameter names the JSON field that holds the status string
// (commonly "status").
func pollForStatus(ctx context.Context, c *client.Client, detailPath string, body map[string]interface{}, statusKey string, targetStatuses []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	targetSet := make(map[string]struct{}, len(targetStatuses))
	for _, s := range targetStatuses {
		targetSet[strings.ToUpper(s)] = struct{}{}
	}
	for {
		resp, _ := callAPI(ctx, c, detailPath, body) // ignore transient errors
		if resp != nil {
			// If the resource is already gone, treat as success (e.g. polling
			// for POWERED_OFF after stop when VM was already deleted).
			if isNotFoundMessage(resp.Message) {
				tflog.Debug(ctx, "pollForStatus: resource not found, treating as done", map[string]interface{}{"path": detailPath})
				return nil
			}
			var data map[string]interface{}
			if err := json.Unmarshal(resp.Data, &data); err == nil {
				status := strings.ToUpper(asString(data, statusKey))
				tflog.Debug(ctx, "pollForStatus tick", map[string]interface{}{"path": detailPath, "status": status, "target": targetStatuses})
				if _, ok := targetSet[status]; ok {
					return nil
				}
				if status == "ERROR" || status == "FAILED" {
					return fmt.Errorf("resource entered error state: %s=%s", statusKey, status)
				}
			} else {
				tflog.Debug(ctx, "pollForStatus: resp.Data unmarshal failed or nil resp", map[string]interface{}{"path": detailPath})
			}
		} else {
			tflog.Debug(ctx, "pollForStatus: callAPI returned nil", map[string]interface{}{"path": detailPath})
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for status %v (timeout=%s)", targetStatuses, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

// pollUntilGone polls a API detail endpoint until it returns a "not found"
// message, the timeout elapses, or ctx is cancelled.
func pollUntilGone(ctx context.Context, c *client.Client, detailPath string, body map[string]interface{}, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		apiResp, diags := callAPI(ctx, c, detailPath, body)
		if diags.HasError() {
			// Any error from the detail call (success=false / code=-1) means
			// the resource no longer exists — deletion complete.
			return nil
		}
		// Also treat as gone when:
		// 1. The API returns success=true but the message indicates not-found.
		// 2. The response data is null or empty (resource record fully purged).
		if apiResp != nil {
			if isNotFoundMessage(apiResp.Message) {
				return nil
			}
			if len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
				return nil
			}
			// Exit if the resource has reached a terminal deleted/deleting state.
			var raw map[string]interface{}
			if err := json.Unmarshal(apiResp.Data, &raw); err == nil {
				if st := strings.ToUpper(asString(raw, "status")); st == "DELETED" || st == "DELETING" {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for resource to be deleted (timeout=%s)", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

// defaultIfEmpty returns the value if it's not null/unknown/empty,
// otherwise returns the default.
func defaultIfEmpty(value types.String, defaultVal string) string {
	if !value.IsNull() && !value.IsUnknown() {
		if s := value.ValueString(); s != "" {
			return s
		}
	}
	return defaultVal
}
