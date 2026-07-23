// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// ---------- Helper: build a minimal LB detail response ----------

func lbDetailResponse(id int64, name, status string) map[string]interface{} {
	return map[string]interface{}{
		"vttLoadBalancerId":       float64(id),
		"name":                    name,
		"description":             "",
		"vttSubnetId":             float64(101),
		"vttFloatingIpId":         float64(0),
		"lbType":                  "Application",
		"vttLoadbalancerTypeName": "Application",
		"packageType":             "STANDARD",
		"loadbalancerTypeName":    "STANDARD",
		"adminStateUp":            true,
		"status":                  status,
		"operatingStatus":         status,
	}
}

func lbListenerResponse() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":              float64(1001),
			"name":            "lb-listener",
			"description":     "",
			"protocol":        "HTTP",
			"protocolPort":    float64(80),
			"xForwardedFor":   false,
			"xForwardedPort":  false,
			"xForwardedProto": false,
		},
	}
}

func lbPoolResponse() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":                     float64(2001),
			"name":                   "lb-pool",
			"description":            "",
			"algorithm":              "ROUND_ROBIN",
			"sessionPersistenceType": "NONE",
		},
	}
}

// ---------- LoadBalancerResource tests ----------

func TestLoadBalancerResource_CreateFlow(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		lb, _ := body["loadBalancer"].(map[string]interface{})
		if lb["name"] != "lb-main" {
			t.Errorf("create: unexpected lb.name %v", lb["name"])
		}
		return float64(0), "ok", map[string]interface{}{"taskId": "task-1"}
	})
	// Poll list endpoint to discover LB ID by name.
	srv.on(pathLoadBalancerList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"items": []map[string]interface{}{
				{"vttLoadBalancerId": float64(42), "name": "lb-main"},
			},
		}
	})
	// pollForStatus calls detail.
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-main", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}

	// Step 1: compound-create.
	createBody := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": r.customerID,
		"loadBalancer": map[string]interface{}{
			"name":   "lb-main",
			"vpcId":  int64(100),
			"lbType": "Application",
		},
		"listener": map[string]interface{}{"name": "lb-main-listener"},
		"pool":     map[string]interface{}{"name": "lb-main-pool"},
		"members":  []map[string]interface{}{},
		"monitor":  map[string]interface{}{"name": "lb-main-health"},
	}
	_, d := callAPI(context.Background(), r.client, pathLoadBalancerCreate, createBody)
	if d.HasError() {
		t.Fatalf("create call failed: %v", d)
	}

	// Step 2: poll list to find ID by name.
	listBody := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": r.customerID,
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
	}
	listResp, d := callAPI(context.Background(), r.client, pathLoadBalancerList, listBody)
	if d.HasError() {
		t.Fatalf("list call failed: %v", d)
	}
	var listResult struct {
		Items []struct {
			VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
			Name              string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Data, &listResult); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	var actualLBID int64
	for _, item := range listResult.Items {
		if item.Name == "lb-main" {
			actualLBID = item.VttLoadBalancerID
		}
	}
	if actualLBID != 42 {
		t.Fatalf("expected lb id=42, got %d", actualLBID)
	}

	// Step 3: readAndMerge.
	model := &LoadBalancerResourceModel{
		ID:    types.StringValue(fmt.Sprintf("%d", actualLBID)),
		VpcID: types.StringValue("100"),
	}
	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), model, &dgs)
	if dgs.HasError() {
		t.Fatalf("readAndMerge: %v", dgs)
	}
	if model.Name.ValueString() != "lb-main" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.Status.ValueString() != "ACTIVE" {
		t.Errorf("status: %q", model.Status.ValueString())
	}
	if model.Listeners.IsNull() || model.Listeners.IsUnknown() {
		t.Error("listeners should be populated")
	}
	if model.Pools.IsNull() || model.Pools.IsUnknown() {
		t.Error("pools should be populated")
	}
}

func TestLoadBalancerResource_Update(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerUpdate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if _, ok := body["vttLoadBalancerId"]; !ok {
			t.Error("update: vttLoadBalancerId missing")
		}
		if _, ok := body["adminStateUp"]; !ok {
			t.Error("update: adminStateUp missing")
		}
		return float64(0), "ok", nil
	})
	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-main", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":            "100",
		"customer_id":       r.customerID,
		"vttLoadBalancerId": int64(42),
		"adminStateUp":      false,
	}
	_, d := callAPI(context.Background(), r.client, pathLoadBalancerUpdate, body)
	if d.HasError() {
		t.Fatalf("update failed: %v", d)
	}
}

func TestLoadBalancerResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if _, ok := body["vttLoadBalancerId"]; !ok {
			t.Error("delete: vttLoadBalancerId missing")
		}
		return float64(0), "ok", nil
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":            "100",
		"customer_id":       r.customerID,
		"vttLoadBalancerId": int64(42),
	}
	_, d := callAPI(context.Background(), r.client, pathLoadBalancerDelete, body)
	if d.HasError() {
		t.Fatalf("delete failed: %v", d)
	}
}

func TestLoadBalancerResource_Delete_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDelete, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "resource not found", nil
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	apiResp, d := callAPI(context.Background(), r.client, pathLoadBalancerDelete, map[string]interface{}{
		"vpc_id":            "100",
		"customer_id":       r.customerID,
		"vttLoadBalancerId": int64(999),
	})
	if d.HasError() && apiResp != nil && isNotFoundMessage(apiResp.Message) {
		return // idempotent — expected
	}
	if d.HasError() {
		t.Fatalf("unexpected error for not-found: %v", d)
	}
}

func TestLoadBalancerResource_ListenersAndPools(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-main", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	model := &LoadBalancerResourceModel{
		ID:    types.StringValue("42"),
		VpcID: types.StringValue("100"),
	}
	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), model, &dgs)
	if dgs.HasError() {
		t.Fatalf("readAndMerge: %v", dgs)
	}

	if len(model.Listeners.Elements()) != 1 {
		t.Errorf("expected 1 listener, got %d", len(model.Listeners.Elements()))
	}
	if len(model.Pools.Elements()) != 1 {
		t.Errorf("expected 1 pool, got %d", len(model.Pools.Elements()))
	}
}

// ---------- LoadBalancerDataSource tests ----------

func TestLoadBalancerDataSource_ByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbDetailResponse(42, "lb-prod", "ACTIVE")
	})
	srv.on(pathLoadBalancerListeners, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbListenerResponse()
	})
	srv.on(pathLoadBalancerPools, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", lbPoolResponse()
	})

	r := &LoadBalancerResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	model := &LoadBalancerResourceModel{
		ID:    types.StringValue("42"),
		VpcID: types.StringValue("100"),
	}
	var dgs diag.Diagnostics
	r.readAndMerge(context.Background(), model, &dgs)
	if dgs.HasError() {
		t.Fatalf("readAndMerge: %v", dgs)
	}
	if model.Name.ValueString() != "lb-prod" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.LoadBalancerType.ValueString() != "Application" {
		t.Errorf("type: %q", model.LoadBalancerType.ValueString())
	}
}

func TestLoadBalancerDataSource_ByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathLoadBalancerList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"items": []map[string]interface{}{
				{"vttLoadBalancerId": float64(10), "name": "lb-dev"},
				{"vttLoadBalancerId": float64(11), "name": "lb-prod"},
			},
		}
	})

	d := &LoadBalancerDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": d.customerID,
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
	}
	apiResp, dgs := callAPI(context.Background(), d.client, pathLoadBalancerList, body)
	if dgs.HasError() {
		t.Fatalf("list: %v", dgs)
	}

	var listResult struct {
		Items []struct {
			VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
			Name              string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResult); err != nil {
		t.Fatalf("decode: %v", err)
	}

	targetName := "lb-prod"
	var foundID int64
	for _, item := range listResult.Items {
		if item.Name == targetName {
			foundID = item.VttLoadBalancerID
			break
		}
	}
	if foundID != 11 {
		t.Errorf("expected id=11, got %d", foundID)
	}
}

// The list endpoint carries ipAddress, provisioningStatus and
// isPublicLoadbalancer; the data source used to drop all three, so looking a
// load balancer up told you nothing about where to point traffic.
func TestLoadBalancerDataSource_LookupByIPAddress(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[
			{"vttLoadBalancerId":928,"name":"other","ipAddress":"10.17.10.10"},
			{"vttLoadBalancerId":929,"name":"dev","ipAddress":"10.17.10.157",
			 "status":"success","operatingStatus":"online","provisioningStatus":"active",
			 "isPublicLoadbalancer":false,"vttSubnetId":9935,
			 "vttLoadbalancerTypeName":"NETWORK TCP-UDP","loadbalancerTypeName":"LB Compact"}
		]}}`))
	}))
	defer srv.Close()

	d := &LoadBalancerDataSource{client: client.NewClient(srv.URL, "tok"), customerID: "238250", defaultVpcID: "39721"}
	cfg := LoadBalancerDataSourceModel{IPAddress: types.StringValue("10.17.10.157")}
	got, diags := d.lookup(context.Background(), &cfg)
	if diags.HasError() {
		t.Fatalf("lookup: %v", diags)
	}
	if got.ID.ValueString() != "929" || got.Name.ValueString() != "dev" {
		t.Fatalf("matched the wrong load balancer: %s/%s", got.ID.ValueString(), got.Name.ValueString())
	}
	if got.ProvisioningStatus.ValueString() != "active" || got.IsPublicLoadBalancer.ValueBool() {
		t.Errorf("new fields not populated: %+v", got)
	}
}

// A NETWORK TCP-UDP load balancer cannot serve HTTP. The API accepts the
// request anyway and builds a load balancer nothing can reach, so the config
// has to be stopped at plan time.
func TestValidateListenerProtocol(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		lbType   string
		protocol string
		wantErr  bool
	}{
		{name: "network with udp", lbType: "NETWORK TCP-UDP", protocol: "UDP"},
		{name: "network with tcp", lbType: "NETWORK TCP-UDP", protocol: "TCP"},
		{name: "network with http", lbType: "NETWORK TCP-UDP", protocol: "HTTP", wantErr: true},
		{name: "application with https", lbType: "APPLICATION HTTP-HTTPS", protocol: "HTTPS"},
		{name: "application with tcp", lbType: "APPLICATION HTTP-HTTPS", protocol: "TCP", wantErr: true},
		{name: "unknown lb type is not second-guessed", lbType: "SOMETHING NEW", protocol: "HTTP"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diags := validateListenerProtocol(types.StringValue(tc.lbType), types.StringValue(tc.protocol))
			if diags.HasError() != tc.wantErr {
				t.Fatalf("lbType=%q protocol=%q: got error=%v, want %v (%v)",
					tc.lbType, tc.protocol, diags.HasError(), tc.wantErr, diags)
			}
		})
	}
}

// Omitting the new attributes has to keep producing the load balancer configs
// written before they existed.
func TestLoadBalancerListenerPoolDefaults(t *testing.T) {
	t.Parallel()
	if got := defaultStr(types.StringNull(), "web-pool"); got != "web-pool" {
		t.Errorf("null should fall back, got %q", got)
	}
	if got := defaultStr(types.StringValue(""), "web-pool"); got != "web-pool" {
		t.Errorf("empty string should fall back, got %q", got)
	}
	if got := defaultStr(types.StringValue("custom"), "web-pool"); got != "custom" {
		t.Errorf("set value should win, got %q", got)
	}
	if got := defaultInt(types.Int64Null(), 80); got != 80 {
		t.Errorf("null port should fall back, got %d", got)
	}
	if got := defaultInt(types.Int64Value(53), 80); got != 53 {
		t.Errorf("set port should win, got %d", got)
	}
}

// certificate_id and TERMINATED_HTTPS are a pair: the cert only terminates TLS,
// and TERMINATED_HTTPS is the only protocol that presents one.
func TestValidateCertificate(t *testing.T) {
	t.Parallel()
	str := types.StringValue
	null := types.StringNull()
	cases := []struct {
		name     string
		protocol types.String
		cert     types.String
		wantErr  bool
	}{
		{name: "terminated with cert", protocol: str("TERMINATED_HTTPS"), cert: str("abc"), wantErr: false},
		{name: "terminated without cert", protocol: str("TERMINATED_HTTPS"), cert: null, wantErr: true},
		{name: "http with cert", protocol: str("HTTP"), cert: str("abc"), wantErr: true},
		{name: "http without cert", protocol: str("HTTP"), cert: null, wantErr: false},
		{name: "tcp without cert", protocol: str("TCP"), cert: null, wantErr: false},
		// cert from another resource is unknown at plan — cannot validate, must not error.
		{name: "terminated with unknown cert", protocol: str("TERMINATED_HTTPS"), cert: types.StringUnknown(), wantErr: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := validateCertificate(tc.protocol, tc.cert).HasError(); got != tc.wantErr {
				t.Fatalf("protocol=%s cert=%v: got error=%v want %v", tc.protocol, tc.cert, got, tc.wantErr)
			}
		})
	}
}
