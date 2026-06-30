// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	sharedpd "terraform-provider-viettelidc/internal/providerdata"
	"terraform-provider-viettelidc/internal/service/vopc/client"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

// callAPI performs a POST or GET to a VKS endpoint, parses the CSA envelope, and returns it.
func normalizeBodyKeys(body map[string]interface{}) {
	if body == nil {
		return
	}
	replacements := map[string]string{
		"clusterId":      "cluster_id",
		"customerId":     "customer_id",
		"hostId":         "host_id",
		"nodeId":         "node_id",
		"groupId":        "group_id",
		"vttClusterId":   "cluster_id",
		"subnetId":       "subnet_id",
		"nicId":          "nic_id",
		"sgId":           "sg_id",
		"startTime":      "start_time",
		"finishTime":     "finish_time",
		"blockStorageId": "block_storage_id",
	}
	for oldKey, newKey := range replacements {
		if val, ok := body[oldKey]; ok {
			body[newKey] = val
			delete(body, oldKey)
		}
	}
}

// callAPI performs a POST or GET to a VKS endpoint, parses the CSA envelope, and returns it.
func callAPI(ctx context.Context, c *client.Client, path string, body map[string]interface{}) (*client.APIResponse, diag.Diagnostics) {
	var diags diag.Diagnostics

	if body != nil {
		normalizeBodyKeys(body)
		if _, ok := body["planType"]; !ok {
			body["planType"] = "k8s"
		}
		if _, ok := body["host_id"]; !ok {
			if c.HostID != 0 {
				body["host_id"] = c.HostID
			}
		}
		isK8s := strings.Contains(path, "/kubernetes/") || strings.Contains(path, "/addon/") || strings.Contains(path, "/download-config")
		for _, k := range []string{"id", "vpc_id", "vpcId", "customer_id", "customerId", "cluster_id", "clusterId", "vttClusterId", "group_id", "groupId", "vttNodeGroupId", "idNodeGroup", "nodeGroupId", "node_group_id"} {
			if v, ok := body[k]; ok {
				if s, isStr := v.(string); isStr {
					if isK8s && (k == "customer_id" || k == "customerId") {
						continue
					}
					if n, err := strconv.Atoi(s); err == nil {
						body[k] = n
					}
				}
			}
		}
	}

	method := "POST"
	if strings.Contains(path, "/iac/") {
		isPost := strings.HasSuffix(path, "/install") ||
			strings.HasSuffix(path, "/uninstall") ||
			strings.HasSuffix(path, "/group/create") ||
			strings.HasSuffix(path, "/group/update") ||
			strings.HasSuffix(path, "/group/delete") ||
			strings.HasSuffix(path, "/nfs/add-ons") ||
			strings.HasSuffix(path, "/kube-config")
		if !isPost {
			method = "GET"
		}
	}

	var raw []byte
	var err error

	if method == "GET" {
		q := url.Values{}
		if body != nil {
			for k, v := range body {
				q.Set(k, fmt.Sprintf("%v", v))
			}
		}
		queryPath := path
		if len(q) > 0 {
			if strings.Contains(path, "?") {
				queryPath += "&" + q.Encode()
			} else {
				queryPath += "?" + q.Encode()
			}
		}
		raw, err = c.DoMethod(ctx, "GET", queryPath, nil)
	} else {
		raw, err = c.Do(ctx, path, body)
	}

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

func asString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
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
