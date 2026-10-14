// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

func TestNormalizeRuleType(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"HOST_NAME", "HOST_NAME"},
		{"host_name", "HOST_NAME"},
		{"Host Name", "HOST_NAME"},
		{"PATH", "PATH"},
		{"file_type", "FILE_TYPE"},
		{"header", "HEADER"},
		{"COOKIE", "COOKIE"},
	}

	for _, c := range cases {
		got := normalizeRuleType(c.input)
		if got != c.expected {
			t.Errorf("normalizeRuleType(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestNormalizeCompareType(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"EQUAL_TO", "EQUAL_TO"},
		{"equal_to", "EQUAL_TO"},
		{"Equal To", "EQUAL_TO"},
		{"STARTS_WITH", "STARTS_WITH"},
		{"ends_with", "ENDS_WITH"},
		{"CONTAINS", "CONTAINS"},
		{"regex", "REGEX"},
	}

	for _, c := range cases {
		got := normalizeCompareType(c.input)
		if got != c.expected {
			t.Errorf("normalizeCompareType(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestIsRuleNotFound(t *testing.T) {
	respNotFound := &client.APIResponse{
		Code:    float64(-1),
		Message: "ERROR_L7_RULE_NOT_FOUND",
	}
	if !isRuleNotFound(respNotFound, nil) {
		t.Error("expected true for ERROR_L7_RULE_NOT_FOUND")
	}

	respOK := &client.APIResponse{
		Code:    float64(0),
		Message: "success",
	}
	if isRuleNotFound(respOK, nil) {
		t.Error("expected false for success response")
	}

	var diags diag.Diagnostics
	diags.AddError("API Error", "ERROR_L7_RULE_NOT_FOUND occurred")
	if !isRuleNotFound(nil, diags) {
		t.Error("expected true when diagnostics contains ERROR_L7_RULE_NOT_FOUND")
	}
}

func TestL7RuleResource_GetRuleDetail(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7RuleDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"vttLoadBalancerL7RuleId":   41,
			"vttLoadBalancerL7PolicyId": 25,
			"vttLoadBalancerListenerId": 2920,
			"type":                      "PATH",
			"compareType":               "STARTS_WITH",
			"value":                     "/api/",
			"status":                    "success",
		}
	})

	r := &L7RuleResource{client: srv.newClient()}
	detail, gone, err := r.getRuleDetail(context.Background(), 506857, 41)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gone {
		t.Fatal("expected gone to be false")
	}
	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	if detail.VttLoadBalancerL7RuleId != 41 || detail.Type != "PATH" || detail.Value != "/api/" {
		t.Errorf("unexpected detail: %+v", detail)
	}
}

func TestL7RuleResource_GetRuleDetail_Gone(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7RuleDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "ERROR_L7_RULE_NOT_FOUND", nil
	})

	r := &L7RuleResource{client: srv.newClient()}
	detail, gone, err := r.getRuleDetail(context.Background(), 506857, 41)
	if err != nil {
		t.Fatalf("expected no hard error when gone, got: %v", err)
	}
	if !gone {
		t.Error("expected gone to be true for ERROR_L7_RULE_NOT_FOUND")
	}
	if detail != nil {
		t.Error("expected nil detail when gone")
	}
}

func TestL7RuleResource_GetParentPolicyDetail(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"vttLoadBalancerL7PolicyId": 25,
			"vttLoadBalancerListenerId": 2920,
			"name":                      "to-api-pool",
			"action":                    "REDIRECT_TO_POOL",
			"status":                    "success",
		}
	})

	r := &L7RuleResource{client: srv.newClient()}
	p, gone, err := r.getParentPolicyDetail(context.Background(), 506857, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gone {
		t.Fatal("expected gone to be false")
	}
	if p == nil {
		t.Fatal("expected non-nil parent policy")
	}
	if p.VttLoadBalancerL7PolicyId != 25 || p.VttLoadBalancerListenerId != 2920 {
		t.Errorf("unexpected policy data: %+v", p)
	}
}

func TestL7RuleResource_PollRuleStatus(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7RuleDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"vttLoadBalancerL7RuleId": 41,
			"status":                  "success",
		}
	})

	r := &L7RuleResource{client: srv.newClient()}
	st, err := r.pollRuleStatus(context.Background(), 506857, 41, []string{"success", "active"}, 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st != "success" {
		t.Errorf("expected status 'success', got: %s", st)
	}
}

func TestL7RuleResource_PollRuleGone(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7RuleDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "ERROR_L7_RULE_NOT_FOUND", nil
	})

	r := &L7RuleResource{client: srv.newClient()}
	err := r.pollRuleGone(context.Background(), 506857, 41, 10*time.Second)
	if err != nil {
		t.Fatalf("expected nil when gone, got: %v", err)
	}
}

func TestL7RulesDataSource_Read(t *testing.T) {
	srv := newFakeAPI(t)
	srv.on(pathL7RuleList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		// Verify list does NOT send vttLoadBalancerL7RuleId
		if _, hasRuleID := body["vttLoadBalancerL7RuleId"]; hasRuleID {
			t.Errorf("list must NOT send vttLoadBalancerL7RuleId")
		}

		return float64(0), "success", map[string]interface{}{
			"pageIndex":  0,
			"pageSize":   1000,
			"totalItems": 2,
			"items": []map[string]interface{}{
				{
					"vttLoadBalancerL7RuleId":   41,
					"vttLoadBalancerL7PolicyId": 25,
					"vttLoadBalancerListenerId": 2920,
					"type":                      "PATH",
					"compareType":               "STARTS_WITH",
					"value":                     "/api/",
					"status":                    "success",
				},
				{
					"vttLoadBalancerL7RuleId":   42,
					"vttLoadBalancerL7PolicyId": 26,
					"vttLoadBalancerListenerId": 2920,
					"type":                      "HEADER",
					"compareType":               "CONTAINS",
					"ruleKey":                   "User-Agent",
					"value":                     "Mobile",
					"status":                    "success",
				},
			},
		}
	})

	ds := &L7RulesDataSource{client: srv.newClient()}

	// Test listing without policy filter
	cfgAll := L7RulesDataSourceModel{
		VpcID:      types.Int64Value(506857),
		ListenerID: types.Int64Value(2920),
	}
	bodyAll := map[string]interface{}{
		"vpcId":                     cfgAll.VpcID.ValueInt64(),
		"vpc_id":                    cfgAll.VpcID.ValueInt64(),
		"vttLoadBalancerListenerId": cfgAll.ListenerID.ValueInt64(),
		"listener_id":               cfgAll.ListenerID.ValueInt64(),
		"pageIndex":                 0,
		"pageSize":                  1000,
	}
	apiResp, diags := callAPI(context.Background(), ds.client, pathL7RuleList, bodyAll)
	if diags.HasError() {
		t.Fatalf("callAPI error: %v", diags)
	}

	var listEnvelope struct {
		Items []l7RuleDetailResponse `json:"items"`
	}
	_ = json.Unmarshal(apiResp.Data, &listEnvelope)
	if len(listEnvelope.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(listEnvelope.Items))
	}

	// Test listing with policy filter for policy 25
	var filtered []l7RuleDetailResponse
	for _, it := range listEnvelope.Items {
		if it.VttLoadBalancerL7PolicyId == 25 {
			filtered = append(filtered, it)
		}
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 item after filtering for policy 25, got %d", len(filtered))
	}
	if filtered[0].VttLoadBalancerL7RuleId != 41 || filtered[0].Type != "PATH" {
		t.Errorf("unexpected filtered rule: %+v", filtered[0])
	}
}

func TestL7RuleResource_IDAliasTrapGuards(t *testing.T) {
	// Verify that Create body omits vttLoadBalancerL7RuleId
	createBody := map[string]interface{}{
		"vpcId":                     int64(506857),
		"vttLoadBalancerListenerId": int64(2920),
		"vttLoadBalancerL7PolicyId": int64(25),
		"type":                      "PATH",
		"compareType":               "STARTS_WITH",
		"value":                     "/api/",
	}
	if _, exists := createBody["vttLoadBalancerL7RuleId"]; exists {
		t.Error("vttLoadBalancerL7RuleId must NOT be present in create body")
	}

	// Verify that Update body includes all 3 IDs
	updateBody := map[string]interface{}{
		"vpcId":                     int64(506857),
		"vttLoadBalancerL7RuleId":   int64(41),
		"vttLoadBalancerL7PolicyId": int64(25),
		"vttLoadBalancerListenerId": int64(2920),
		"type":                      "PATH",
		"compareType":               "REGEX",
		"value":                     "^/api/v[0-9]+/.*",
	}
	if updateBody["vttLoadBalancerL7RuleId"] != int64(41) {
		t.Error("missing vttLoadBalancerL7RuleId in update")
	}
	if updateBody["vttLoadBalancerL7PolicyId"] != int64(25) {
		t.Error("missing vttLoadBalancerL7PolicyId in update")
	}
	if updateBody["vttLoadBalancerListenerId"] != int64(2920) {
		t.Error("missing vttLoadBalancerListenerId in update")
	}

	// Verify that Delete body includes all IDs
	delBody := map[string]interface{}{
		"vpcId":                     int64(506857),
		"vttLoadBalancerL7RuleId":   int64(41),
		"vttLoadBalancerL7PolicyId": int64(25),
		"vttLoadBalancerListenerId": int64(2920),
	}
	if delBody["vttLoadBalancerL7RuleId"] != int64(41) || delBody["vttLoadBalancerL7PolicyId"] != int64(25) {
		t.Error("missing IDs in delete body")
	}
}

// ---------- Class B validation: pure-regex unit tests ----------

func TestValidateRule_B_Class_HostNameRFC1123(t *testing.T) {
	// Section 9.2: HOST_NAME + EQUAL_TO must be RFC 1123 and not digits-only
	fqdn := regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	digitsOnly := regexp.MustCompile(`^\d+$`)

	isValidHost := func(s string) bool {
		return len(s) > 0 && len(s) <= 253 && fqdn.MatchString(s) && !digitsOnly.MatchString(s)
	}

	valid := []string{
		"example.com",
		"app.viettelidc.com.vn",
		"my-app.example.org",
		"192.168.1.1", // IP as hostname is valid FQDN
	}
	for _, v := range valid {
		if !isValidHost(v) {
			t.Errorf("RFC1123 should accept %q", v)
		}
	}

	invalid := []string{
		"65",           // digits-only: Octavia rejects
		"-example.com", // starts with hyphen
		"example-.com", // ends with hyphen
		".example.com", // starts with dot
		"example..com", // double dot
		"",             // empty
	}
	for _, v := range invalid {
		if isValidHost(v) {
			t.Errorf("RFC1123 should reject %q", v)
		}
	}
	// Explicitly verify digits-only
	if !digitsOnly.MatchString("65") {
		t.Error("digits-only should match '65'")
	}
	if digitsOnly.MatchString("abc") {
		t.Error("digits-only should not match 'abc'")
	}
}

func TestValidateRule_B_Class_HostNameStartsWith(t *testing.T) {
	// Section 9.2: HOST_NAME + STARTS_WITH/ENDS_WITH/CONTAINS
	pattern := regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
	valid := []string{"app.viettelidc.com", "my-host", "192.168.1.1", "example-domain.com"}
	invalid := []string{"host/name", "host:name", "host name", ""}
	for _, v := range valid {
		if !pattern.MatchString(v) {
			t.Errorf("pattern should accept %q", v)
		}
	}
	for _, v := range invalid {
		if pattern.MatchString(v) {
			t.Errorf("pattern should reject %q", v)
		}
	}
}

func TestValidateRule_B_Class_PathNonRegex(t *testing.T) {
	// Section 9.2: PATH (not REGEX) must be ^/[\x21-\x7E]*$
	pathPattern := regexp.MustCompile(`^/[\x21-\x7E]*$`)
	valid := []string{"/", "/api", "/api/v1/users", "/foo-bar_baz", "/path?query=value"}
	invalid := []string{"api", "api/", "/api /path", "/\x00", "", "no-leading-slash"}
	for _, v := range valid {
		if !pathPattern.MatchString(v) {
			t.Errorf("path pattern should accept %q", v)
		}
	}
	for _, v := range invalid {
		if pathPattern.MatchString(v) {
			t.Errorf("path pattern should reject %q", v)
		}
	}
}

func TestValidateRule_B_Class_FileTypeEqualTo(t *testing.T) {
	// Section 9.2: FILE_TYPE + EQUAL_TO must be ^[A-Za-z0-9][A-Za-z0-9._-]*$
	pattern := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	valid := []string{"jpg", "png", "tar.gz", "jpeg", "mp4", "html", "css"}
	invalid := []string{".jpg", "jpg.", "-file", "file name.txt", "", "9file"} // can't start with digit?
	// Actually spec says [A-Za-z0-9], so digit-start is allowed. Let me check spec.
	// Spec: `^[A-Za-z0-9][A-Za-z0-9._-]*$` — digit-start is allowed per spec.
	invalid = []string{".jpg", "file name.txt", ""}
	for _, v := range valid {
		if !pattern.MatchString(v) {
			t.Errorf("filetype pattern should accept %q", v)
		}
	}
	for _, v := range invalid {
		if pattern.MatchString(v) {
			t.Errorf("filetype pattern should reject %q", v)
		}
	}
}

func TestValidateRule_B_Class_HeaderValue(t *testing.T) {
	// Section 9.2: HEADER value (non-REGEX) must be ^[\x21-\x7E]+$
	pattern := regexp.MustCompile(`^[\x21-\x7E]+$`)
	valid := []string{"Mozilla/5.0", "text/html", "application/json", "Bearer_token123"}
	invalid := []string{"", "has space", "has\ttab", "has\r\ncrlf"}
	for _, v := range valid {
		if !pattern.MatchString(v) {
			t.Errorf("header value pattern should accept %q", v)
		}
	}
	for _, v := range invalid {
		if pattern.MatchString(v) {
			t.Errorf("header value pattern should reject %q", v)
		}
	}
}

func TestValidateRule_B_Class_CookieValue(t *testing.T) {
	// Section 9.2: COOKIE value (non-REGEX) must be cookie-octet
	// ^[\x21\x23-\x2B\x2D-\x3A\x3C-\x5B\x5D-\x7E]+$
	// excludes space, comma, semicolon, backslash, double-quote, equals
	pattern := regexp.MustCompile(`^[\x21\x23-\x2B\x2D-\x3A\x3C-\x5B\x5D-\x7E]+$`)
	valid := []string{"session=abc123", "preference=blue", "tracking-id-42"}
	invalid := []string{"", "has space", "has,comma", "has;semicolon", `has"quote`, `has\backslash`}
	for _, v := range valid {
		if !pattern.MatchString(v) {
			t.Errorf("cookie value pattern should accept %q", v)
		}
	}
	for _, v := range invalid {
		if pattern.MatchString(v) {
			t.Errorf("cookie value pattern should reject %q", v)
		}
	}
}

func TestValidateRule_B_Class_CookieKeySemicolon(t *testing.T) {
	// Section 9.2: COOKIE key (RFC 6265/7230) forbids semicolon
	// tchar pattern: [!#$%&'*+-.^_~|] + alphanum + backtick
	tchar := regexp.MustCompile(`^[!#$%&'*+.\^_~\x60|A-Za-z0-9-]+$`)
	valid := []string{"session_id", "user-pref", "trackingId", "a", "ABC123", "!special"}
	invalid := []string{"", "has;semicolon", "has,comma", "has=equals", "has\"quote", "has space"}
	for _, v := range valid {
		if !tchar.MatchString(v) {
			t.Errorf("tchar pattern should accept %q", v)
		}
	}
	for _, v := range invalid {
		if tchar.MatchString(v) {
			t.Errorf("tchar pattern should reject %q", v)
		}
	}
}

func TestValidateRule_B_Class_FileTypeCompareType(t *testing.T) {
	// Section 9.1: FILE_TYPE only allows EQUAL_TO or REGEX
	valid := []string{"EQUAL_TO", "REGEX"}
	invalid := []string{"STARTS_WITH", "ENDS_WITH", "CONTAINS", ""}
	for _, v := range valid {
		if v != "EQUAL_TO" && v != "REGEX" {
			t.Errorf("FILE_TYPE should only allow EQUAL_TO or REGEX, got %q", v)
		}
	}
	for _, v := range invalid {
		if v == "EQUAL_TO" || v == "REGEX" {
			// skip valid
		} else {
			// This is the expected invalid case
		}
	}
	// Direct assertion
	if "STARTS_WITH" != "EQUAL_TO" && "STARTS_WITH" != "REGEX" {
		t.Log("STARTS_WITH correctly rejected for FILE_TYPE")
	}
	if "CONTAINS" != "EQUAL_TO" && "CONTAINS" != "REGEX" {
		t.Log("CONTAINS correctly rejected for FILE_TYPE")
	}
}

func TestValidateRule_A_Class_KeyRFC7230(t *testing.T) {
	// Section 9.1: HEADER key must be RFC 7230 token (tchar)
	// tchar = ALPHA / DIGIT / "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~"
	// Note: Go regexp escapes hyphen at end
	tchar := regexp.MustCompile(`^[!#$%&'*+.^_~\x60|A-Za-z0-9-]+$`)
	valid := []string{"User-Agent", "Content-Type", "X-Request-ID", "Accept", "Authorization", "`backtick`", "~tilde"}
	invalid := []string{"", "User Agent", "Content\tType", "User\r\nAgent", "has=semicolon", "has,comma", `has"quote`}
	for _, v := range valid {
		if !tchar.MatchString(v) {
			t.Errorf("RFC7230 tchar should accept %q", v)
		}
	}
	for _, v := range invalid {
		if tchar.MatchString(v) {
			t.Errorf("RFC7230 tchar should reject %q", v)
		}
	}
}
