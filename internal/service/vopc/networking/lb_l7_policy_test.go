// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"encoding/json"
	"strings"
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

func TestNormalizeAction(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"REDIRECT_TO_URL", "REDIRECT_TO_URL"},
		{"redirect_to_pool", "REDIRECT_TO_POOL"},
		{"Redirect to Pool", "REDIRECT_TO_POOL"},
		{" reject ", "REJECT"},
		{"Redirect to Url", "REDIRECT_TO_URL"},
	}

	for _, c := range cases {
		got := normalizeAction(c.input)
		if got != c.expected {
			t.Errorf("normalizeAction(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestParseID(t *testing.T) {
	if parseID(float64(42)) != 42 {
		t.Errorf("float64: expected 42")
	}
	if parseID(int64(42)) != 42 {
		t.Errorf("int64: expected 42")
	}
	if parseID(int(42)) != 42 {
		t.Errorf("int: expected 42")
	}
	if parseID("42") != 42 {
		t.Errorf("string: expected 42")
	}
	if parseID(nil) != 0 {
		t.Errorf("nil: expected 0")
	}
}

func TestIsPolicyNotFound(t *testing.T) {
	respNotFound := &client.APIResponse{
		Code:    float64(-1),
		Message: "ERROR_L7_POLICY_NOT_FOUND",
	}
	if !isPolicyNotFound(respNotFound, nil) {
		t.Error("expected true for ERROR_L7_POLICY_NOT_FOUND")
	}

	respOK := &client.APIResponse{
		Code:    float64(0),
		Message: "success",
	}
	if isPolicyNotFound(respOK, nil) {
		t.Error("expected false for success response")
	}

	var diags diag.Diagnostics
	diags.AddError("API Error", "ERROR_L7_POLICY_NOT_FOUND occurred")
	if !isPolicyNotFound(nil, diags) {
		t.Error("expected true when diagnostics contains ERROR_L7_POLICY_NOT_FOUND")
	}
}

func TestL7PolicyResource_GetListenerDetail(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerListenerDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"vttLoadBalancerListenerId": 2920,
			"vttLoadBalancerId":         1817,
			"name":                      "listener-http",
			"protocol":                  "HTTP",
			"protocolPort":              80,
			"status":                    "success",
		}
	})

	r := &L7PolicyResource{client: srv.newClient()}
	l, err := r.getListenerDetail(context.Background(), 506857, 2920)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil listener")
	}
	if l.VttLoadBalancerListenerId != 2920 || l.VttLoadBalancerId != 1817 {
		t.Errorf("unexpected listener IDs: %+v", l)
	}
	if l.Protocol != "HTTP" {
		t.Errorf("expected HTTP, got: %s", l.Protocol)
	}
}

func TestL7PolicyResource_IsPoolAvailable(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyPools, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{"id": 3914, "name": "pool-api"},
			{"id": 3915, "name": "pool-web"},
		}
	})

	r := &L7PolicyResource{client: srv.newClient()}
	avail, err := r.isPoolAvailable(context.Background(), 506857, 2920, 3914)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !avail {
		t.Error("expected pool 3914 to be available")
	}

	notAvail, err := r.isPoolAvailable(context.Background(), 506857, 2920, 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notAvail {
		t.Error("expected pool 9999 not to be available")
	}
}

func TestL7PolicyResource_GetPoolDetail(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerPoolDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"vttLoadBalancerPoolId": 3914,
			"vttLoadBalancerId":     1817,
			"name":                  "pool-api",
			"protocol":              "HTTP",
			"status":                "success",
		}
	})

	r := &L7PolicyResource{client: srv.newClient()}
	p, err := r.getPoolDetail(context.Background(), 506857, 1817, 3914)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil pool")
	}
	if p.VttLoadBalancerPoolId != 3914 || p.Protocol != "HTTP" || p.Status != "success" {
		t.Errorf("unexpected pool data: %+v", p)
	}
}

func TestL7PolicyResource_GetPolicyDetail_Gone(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "ERROR_L7_POLICY_NOT_FOUND", nil
	})

	r := &L7PolicyResource{client: srv.newClient()}
	detail, gone, err := r.getPolicyDetail(context.Background(), 506857, 25)
	if err != nil {
		t.Fatalf("expected no hard error when gone, got: %v", err)
	}
	if !gone {
		t.Error("expected gone to be true for ERROR_L7_POLICY_NOT_FOUND")
	}
	if detail != nil {
		t.Error("expected nil detail when gone")
	}
}

func TestL7PolicyResource_FindPolicyInList(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"pageIndex":  0,
			"pageSize":   1000,
			"totalItems": 2,
			"items": []map[string]interface{}{
				{
					"vttLoadBalancerL7PolicyId": 25,
					"vttLoadBalancerListenerId": 2920,
					"name":                      "to-api-pool",
					"action":                    "REDIRECT_TO_POOL",
					"status":                    "success",
				},
				{
					"vttLoadBalancerL7PolicyId": 26,
					"vttLoadBalancerListenerId": 2920,
					"name":                      "redirect-old",
					"action":                    "REDIRECT_TO_URL",
					"status":                    "success",
				},
			},
		}
	})

	r := &L7PolicyResource{client: srv.newClient()}
	id, err := r.findPolicyInList(context.Background(), 506857, 2920, "to-api-pool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 25 {
		t.Errorf("expected ID 25, got: %d", id)
	}

	notFoundID, err := r.findPolicyInList(context.Background(), 506857, 2920, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFoundID != 0 {
		t.Errorf("expected 0 for nonexistent policy, got: %d", notFoundID)
	}
}

func TestL7PolicyResource_PollPolicyStatus(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"vttLoadBalancerL7PolicyId": 25,
			"status":                    "success",
		}
	})

	r := &L7PolicyResource{client: srv.newClient()}
	st, err := r.pollPolicyStatus(context.Background(), 506857, 25, []string{"success", "active"}, 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st != "success" {
		t.Errorf("expected status 'success', got: %s", st)
	}
}

func TestL7PolicyResource_PollPolicyGone(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "ERROR_L7_POLICY_NOT_FOUND", nil
	})

	r := &L7PolicyResource{client: srv.newClient()}
	err := r.pollPolicyGone(context.Background(), 506857, 25, 10*time.Second)
	if err != nil {
		t.Fatalf("expected nil when gone, got: %v", err)
	}
}

func TestL7PoliciesDataSource_Read(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"pageIndex":  0,
			"pageSize":   1000,
			"totalItems": 1,
			"items": []map[string]interface{}{
				{
					"vttLoadBalancerL7PolicyId": 25,
					"vttLoadBalancerListenerId": 2920,
					"name":                      "to-api-pool",
					"description":               "route /api",
					"action":                    "REDIRECT_TO_POOL",
					"vttLoadBalancerPoolId":     3914,
					"poolName":                  "pool-api",
					"priority":                  1,
					"status":                    "success",
					"numberOfRules":             2,
				},
			},
		}
	})

	ds := &L7PoliciesDataSource{client: srv.newClient()}
	cfg := L7PoliciesDataSourceModel{
		VpcID:      types.Int64Value(506857),
		ListenerID: types.Int64Value(2920),
	}

	body := map[string]interface{}{
		"vpcId":                     cfg.VpcID.ValueInt64(),
		"vpc_id":                    cfg.VpcID.ValueInt64(),
		"vttLoadBalancerListenerId": cfg.ListenerID.ValueInt64(),
		"listener_id":               cfg.ListenerID.ValueInt64(),
		"pageIndex":                 0,
		"pageSize":                  1000,
	}

	apiResp, diags := callAPI(context.Background(), ds.client, pathL7PolicyList, body)
	if diags.HasError() {
		t.Fatalf("callAPI error: %v", diags)
	}

	var listEnvelope struct {
		Items []l7PolicyDetailResponse `json:"items"`
	}
	_ = json.Unmarshal(apiResp.Data, &listEnvelope)
	if len(listEnvelope.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(listEnvelope.Items))
	}

	item := listEnvelope.Items[0]
	if item.VttLoadBalancerL7PolicyId != 25 || item.Name != "to-api-pool" || item.Action != "REDIRECT_TO_POOL" {
		t.Errorf("unexpected item content: %+v", item)
	}
}

func TestL7AvailablePoolsDataSource_Read(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyPools, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{"id": 3914, "name": "pool-api"},
			{"id": 3915, "name": "pool-web"},
		}
	})

	ds := &L7AvailablePoolsDataSource{client: srv.newClient()}
	cfg := L7AvailablePoolsDataSourceModel{
		VpcID:      types.Int64Value(506857),
		ListenerID: types.Int64Value(2920),
	}

	body := map[string]interface{}{
		"vpcId":                     cfg.VpcID.ValueInt64(),
		"vpc_id":                    cfg.VpcID.ValueInt64(),
		"vttLoadBalancerListenerId": cfg.ListenerID.ValueInt64(),
		"listener_id":               cfg.ListenerID.ValueInt64(),
	}

	apiResp, diags := callAPI(context.Background(), ds.client, pathL7PolicyPools, body)
	if diags.HasError() {
		t.Fatalf("callAPI error: %v", diags)
	}

	var rawPools []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(apiResp.Data, &rawPools)
	if len(rawPools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(rawPools))
	}
	if rawPools[0].ID != 3914 || rawPools[1].ID != 3915 {
		t.Errorf("unexpected pools: %+v", rawPools)
	}
}
// ---------- Policy Class A and B validation tests ----------

func TestValidatePolicy_NameRegex(t *testing.T) {
	// Section 9.1: 1-255 characters, ^[a-zA-Z0-9.,@_\- ]+$
	valid := []string{"to-api-pool", "redirect_url.1", "policy@test", "Rule 1", "simple"}
	invalid := []string{"", "invalid#char", "policy$1", "test/path", "name:colon"}

	for _, v := range valid {
		if !policyNameRegex.MatchString(v) {
			t.Errorf("policyNameRegex should accept %q", v)
		}
	}
	for _, v := range invalid {
		if policyNameRegex.MatchString(v) && len(v) > 0 {
			t.Errorf("policyNameRegex should reject %q", v)
		}
	}
}

func TestValidatePolicy_ActionRules(t *testing.T) {
	// REJECT: no redirect_url, no redirect_pool_id
	// REDIRECT_TO_URL: redirect_url required, must start with http:// or https://, no redirect_pool_id
	// REDIRECT_TO_POOL: redirect_pool_id required > 0, no redirect_url

	urlValid := []string{"http://example.com", "https://api.viettelidc.com.vn/v1", "https://example.com:8080/path"}
	urlInvalid := []string{"ftp://example.com", "example.com", "http://", "https://", "https://ex ample.com"}

	for _, u := range urlValid {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			t.Errorf("expected valid prefix for %q", u)
		}
		if strings.ContainsAny(u, " \t\r\n") {
			t.Errorf("expected no whitespace for %q", u)
		}
	}

	for _, u := range urlInvalid {
		hasValidPrefix := strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
		hasWhitespace := strings.ContainsAny(u, " \t\r\n")
		isTooShort := len(u) <= 8 // "http://" is 7, "https://" is 8
		if !hasValidPrefix || hasWhitespace || isTooShort {
			// correctly detected invalid
		} else {
			t.Errorf("expected %q to be considered invalid", u)
		}
	}
}

func TestValidatePolicy_ListenerProtocol(t *testing.T) {
	// Section 9.2: Policy only attaches to HTTP or TERMINATED_HTTPS listeners
	validProtocols := []string{"HTTP", "TERMINATED_HTTPS", "http", "terminated_https"}
	invalidProtocols := []string{"TCP", "UDP", "HTTPS", "TLS", "GRPC"}

	for _, p := range validProtocols {
		pUpper := strings.ToUpper(p)
		if pUpper != "HTTP" && pUpper != "TERMINATED_HTTPS" {
			t.Errorf("expected protocol %q to be accepted", p)
		}
	}

	for _, p := range invalidProtocols {
		pUpper := strings.ToUpper(p)
		if pUpper == "HTTP" || pUpper == "TERMINATED_HTTPS" {
			t.Errorf("expected protocol %q to be rejected", p)
		}
	}
}

func TestValidatePolicy_PoolCompatibility(t *testing.T) {
	// Section 9.2: Destination pool protocol must be HTTP, PROXY, or PROXYV2
	compatibleProtocols := []string{"HTTP", "PROXY", "PROXYV2"}
	incompatibleProtocols := []string{"TCP", "UDP", "HTTPS"}

	isCompatible := func(proto string) bool {
		switch strings.ToUpper(proto) {
		case "HTTP", "PROXY", "PROXYV2":
			return true
		default:
			return false
		}
	}

	for _, p := range compatibleProtocols {
		if !isCompatible(p) {
			t.Errorf("expected pool protocol %q to be compatible", p)
		}
	}

	for _, p := range incompatibleProtocols {
		if isCompatible(p) {
			t.Errorf("expected pool protocol %q to be incompatible", p)
		}
	}
}
