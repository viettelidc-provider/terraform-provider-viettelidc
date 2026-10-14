// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestL7PolicyDataSource_Schema(t *testing.T) {
	t.Parallel()
	ds := NewL7PolicyDataSource()
	var req datasource.SchemaRequest
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema error: %v", resp.Diagnostics)
	}

	attrs := resp.Schema.Attributes
	for _, expected := range []string{"vpc_id", "listener_id", "id", "name", "description", "action", "redirect_url", "redirect_pool_id", "priority", "status", "number_of_rules"} {
		if _, ok := attrs[expected]; !ok {
			t.Errorf("missing expected attribute %q in schema", expected)
		}
	}
}

func TestL7PolicyDataSource_LookupByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"vttLoadBalancerL7PolicyId": 101,
					"vttLoadBalancerListenerId": 2001,
					"name":                      "policy-api",
					"description":               "route /api traffic",
					"action":                    "REDIRECT_TO_POOL",
					"vttLoadBalancerPoolId":     3001,
					"priority":                  1,
					"status":                    "ACTIVE",
					"numberOfRules":             3,
				},
				{
					"vttLoadBalancerL7PolicyId": 102,
					"vttLoadBalancerListenerId": 2001,
					"name":                      "policy-static",
					"description":               "redirect static",
					"action":                    "REDIRECT_TO_URL",
					"redirectUrl":               "https://cdn.example.com",
					"status":                    "ACTIVE",
					"numberOfRules":             1,
				},
			},
		}
	})

	ds := &L7PolicyDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}
	cfg := &L7PolicyDataSourceModel{
		VpcID:      types.Int64Value(10),
		ListenerID: types.Int64Value(2001),
		Name:       types.StringValue("policy-api"),
	}

	res, diags := ds.lookup(context.Background(), cfg)
	if diags.HasError() {
		t.Fatalf("lookup failed: %v", diags)
	}

	if res.ID.ValueString() != "101" {
		t.Errorf("got ID=%q, want 101", res.ID.ValueString())
	}
	if res.Name.ValueString() != "policy-api" {
		t.Errorf("got Name=%q, want policy-api", res.Name.ValueString())
	}
	if res.Action.ValueString() != "REDIRECT_TO_POOL" {
		t.Errorf("got Action=%q, want REDIRECT_TO_POOL", res.Action.ValueString())
	}
	if res.RedirectPoolID.ValueInt64() != 3001 {
		t.Errorf("got RedirectPoolID=%d, want 3001", res.RedirectPoolID.ValueInt64())
	}
	if res.Priority.ValueInt64() != 1 {
		t.Errorf("got Priority=%d, want 1", res.Priority.ValueInt64())
	}
	if res.Status.ValueString() != "active" {
		t.Errorf("got Status=%q, want active", res.Status.ValueString())
	}
	if res.NumberOfRules.ValueInt64() != 3 {
		t.Errorf("got NumberOfRules=%d, want 3", res.NumberOfRules.ValueInt64())
	}
}

func TestL7PolicyDataSource_LookupByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathL7PolicyList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"vttLoadBalancerL7PolicyId": 102,
					"vttLoadBalancerListenerId": 2001,
					"name":                      "policy-static",
					"description":               "redirect static",
					"action":                    "REDIRECT_TO_URL",
					"redirectUrl":               "https://cdn.example.com",
					"status":                    "SUCCESS",
					"numberOfRules":             1,
				},
			},
		}
	})

	ds := &L7PolicyDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}
	cfg := &L7PolicyDataSourceModel{
		VpcID:      types.Int64Value(10),
		ListenerID: types.Int64Value(2001),
		ID:         types.StringValue("102"),
	}

	res, diags := ds.lookup(context.Background(), cfg)
	if diags.HasError() {
		t.Fatalf("lookup failed: %v", diags)
	}

	if res.Name.ValueString() != "policy-static" {
		t.Errorf("got Name=%q, want policy-static", res.Name.ValueString())
	}
	if res.RedirectURL.ValueString() != "https://cdn.example.com" {
		t.Errorf("got RedirectURL=%q, want https://cdn.example.com", res.RedirectURL.ValueString())
	}
	if res.Status.ValueString() != "success" {
		t.Errorf("got Status=%q, want success", res.Status.ValueString())
	}
}

func TestL7PolicyDataSource_Validation(t *testing.T) {
	t.Parallel()
	ds := &L7PolicyDataSource{}

	// Neither ID nor Name set
	cfgNeither := &L7PolicyDataSourceModel{
		VpcID:      types.Int64Value(10),
		ListenerID: types.Int64Value(2001),
	}
	_, diags := ds.lookup(context.Background(), cfgNeither)
	if !diags.HasError() {
		t.Error("expected error when neither ID nor Name is set")
	}

	// Both ID and Name set
	cfgBoth := &L7PolicyDataSourceModel{
		VpcID:      types.Int64Value(10),
		ListenerID: types.Int64Value(2001),
		ID:         types.StringValue("101"),
		Name:       types.StringValue("policy-api"),
	}
	_, diags = ds.lookup(context.Background(), cfgBoth)
	if !diags.HasError() {
		t.Error("expected error when both ID and Name are set")
	}
}

func TestLBListenerDataSource_Schema(t *testing.T) {
	t.Parallel()
	ds := NewLBListenerDataSource()
	var req datasource.SchemaRequest
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema error: %v", resp.Diagnostics)
	}

	attrs := resp.Schema.Attributes
	for _, expected := range []string{"vpc_id", "load_balancer_id", "load_balancer_name", "id", "name", "port", "protocol", "status", "default_pool_id"} {
		if _, ok := attrs[expected]; !ok {
			t.Errorf("missing expected attribute %q in schema", expected)
		}
	}
}

func TestLBListenerDataSource_LookupByNameAndPort(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerListeners, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{
				"vttLoadBalancerListenerId": 2001,
				"name":                      "http-listener",
				"protocol":                  "HTTP",
				"protocolPort":              80,
				"status":                    "ACTIVE",
				"defaultPoolId":             3001,
			},
			{
				"vttLoadBalancerListenerId": 2002,
				"name":                      "https-listener",
				"protocol":                  "HTTPS",
				"protocolPort":              443,
				"status":                    "ACTIVE",
				"defaultPoolId":             3002,
			},
		}
	})

	ds := &LBListenerDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}

	// Lookup by name
	cfgByName := &LBListenerDataSourceModel{
		VpcID:          types.Int64Value(10),
		LoadBalancerID: types.Int64Value(500),
		Name:           types.StringValue("https-listener"),
	}
	res, diags := ds.lookup(context.Background(), cfgByName)
	if diags.HasError() {
		t.Fatalf("lookup by name failed: %v", diags)
	}
	if res.ID.ValueInt64() != 2002 {
		t.Errorf("got ID=%d, want 2002", res.ID.ValueInt64())
	}
	if res.Port.ValueInt64() != 443 {
		t.Errorf("got Port=%d, want 443", res.Port.ValueInt64())
	}
	if res.Protocol.ValueString() != "HTTPS" {
		t.Errorf("got Protocol=%q, want HTTPS", res.Protocol.ValueString())
	}
	if res.DefaultPoolID.ValueInt64() != 3002 {
		t.Errorf("got DefaultPoolID=%d, want 3002", res.DefaultPoolID.ValueInt64())
	}

	// Lookup by port
	cfgByPort := &LBListenerDataSourceModel{
		VpcID:          types.Int64Value(10),
		LoadBalancerID: types.Int64Value(500),
		Port:           types.Int64Value(80),
	}
	res2, diags2 := ds.lookup(context.Background(), cfgByPort)
	if diags2.HasError() {
		t.Fatalf("lookup by port failed: %v", diags2)
	}
	if res2.Name.ValueString() != "http-listener" {
		t.Errorf("got Name=%q, want http-listener", res2.Name.ValueString())
	}
	if res2.ID.ValueInt64() != 2001 {
		t.Errorf("got ID=%d, want 2001", res2.ID.ValueInt64())
	}
}

func TestLBListenerDataSource_PoolCorrelation(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	// Listeners list without defaultPoolId
	srv.on(pathLoadBalancerListeners, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{
				"vttLoadBalancerListenerId": 2001,
				"name":                      "http-listener",
				"protocol":                  "HTTP",
				"protocolPort":              80,
				"status":                    "ACTIVE",
			},
		}
	})
	// Pools endpoint has listener correlation
	srv.on(pathLoadBalancerPools, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{
				"vttLoadBalancerPoolId":     3001,
				"vttLoadBalancerListenerId": 2001,
				"name":                      "pool-web",
			},
		}
	})

	ds := &LBListenerDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}
	cfg := &LBListenerDataSourceModel{
		VpcID:          types.Int64Value(10),
		LoadBalancerID: types.Int64Value(500),
		Name:           types.StringValue("http-listener"),
	}

	res, diags := ds.lookup(context.Background(), cfg)
	if diags.HasError() {
		t.Fatalf("lookup failed: %v", diags)
	}
	if res.DefaultPoolID.ValueInt64() != 3001 {
		t.Errorf("got DefaultPoolID=%d, want 3001", res.DefaultPoolID.ValueInt64())
	}
}

func TestLBPoolDataSource_Schema(t *testing.T) {
	t.Parallel()
	ds := NewLBPoolDataSource()
	var req datasource.SchemaRequest
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema error: %v", resp.Diagnostics)
	}

	attrs := resp.Schema.Attributes
	for _, expected := range []string{"vpc_id", "load_balancer_id", "load_balancer_name", "id", "name", "protocol", "algorithm", "status"} {
		if _, ok := attrs[expected]; !ok {
			t.Errorf("missing expected attribute %q in schema", expected)
		}
	}
}

func TestLBPoolDataSource_LookupByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerPools, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{
				"vttLoadBalancerPoolId": 3001,
				"vttLoadBalancerId":     500,
				"name":                  "pool-web",
				"protocol":              "HTTP",
				"algorithm":             "ROUND_ROBIN",
				"status":                "ACTIVE",
			},
			{
				"vttLoadBalancerPoolId": 3002,
				"vttLoadBalancerId":     500,
				"name":                  "pool-api",
				"protocol":              "HTTP",
				"algorithm":             "LEAST_CONNECTIONS",
				"status":                "ACTIVE",
			},
		}
	})

	ds := &LBPoolDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}
	cfg := &LBPoolDataSourceModel{
		VpcID:          types.Int64Value(10),
		LoadBalancerID: types.Int64Value(500),
		Name:           types.StringValue("pool-api"),
	}

	res, diags := ds.lookup(context.Background(), cfg)
	if diags.HasError() {
		t.Fatalf("lookup failed: %v", diags)
	}

	if res.ID.ValueInt64() != 3002 {
		t.Errorf("got ID=%d, want 3002", res.ID.ValueInt64())
	}
	if res.Name.ValueString() != "pool-api" {
		t.Errorf("got Name=%q, want pool-api", res.Name.ValueString())
	}
	if res.Algorithm.ValueString() != "LEAST_CONNECTIONS" {
		t.Errorf("got Algorithm=%q, want LEAST_CONNECTIONS", res.Algorithm.ValueString())
	}
	if res.Protocol.ValueString() != "HTTP" {
		t.Errorf("got Protocol=%q, want HTTP", res.Protocol.ValueString())
	}
	if res.Status.ValueString() != "active" {
		t.Errorf("got Status=%q, want active", res.Status.ValueString())
	}
}

func TestLBPoolDataSource_LookupByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerPools, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{
				"id":        3001,
				"name":      "pool-web",
				"protocol":  "HTTP",
				"algorithm": "ROUND_ROBIN",
				"status":    "ACTIVE",
			},
		}
	})

	ds := &LBPoolDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}
	cfg := &LBPoolDataSourceModel{
		VpcID:          types.Int64Value(10),
		LoadBalancerID: types.Int64Value(500),
		ID:             types.Int64Value(3001),
	}

	res, diags := ds.lookup(context.Background(), cfg)
	if diags.HasError() {
		t.Fatalf("lookup failed: %v", diags)
	}

	if res.Name.ValueString() != "pool-web" {
		t.Errorf("got Name=%q, want pool-web", res.Name.ValueString())
	}
}

func TestLBListenerDataSource_LookupByLBName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"items": []map[string]interface{}{
				{"vttLoadBalancerId": 500, "name": "my-lb"},
			},
		}
	})
	srv.on(pathLoadBalancerListeners, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{
				"vttLoadBalancerListenerId": 2002,
				"name":                      "https-listener",
				"protocol":                  "HTTPS",
				"protocolPort":              443,
				"status":                    "ACTIVE",
			},
		}
	})

	ds := &LBListenerDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}

	// Lookup using only load_balancer_name (load_balancer_id is null)
	cfgByLBName := &LBListenerDataSourceModel{
		VpcID:            types.Int64Value(10),
		LoadBalancerName: types.StringValue("my-lb"),
		Name:             types.StringValue("https-listener"),
	}
	res, diags := ds.lookup(context.Background(), cfgByLBName)
	if diags.HasError() {
		t.Fatalf("lookup by LB name failed: %v", diags)
	}
	if res.LoadBalancerID.ValueInt64() != 500 {
		t.Errorf("got LoadBalancerID=%d, want 500", res.LoadBalancerID.ValueInt64())
	}
	if res.ID.ValueInt64() != 2002 {
		t.Errorf("got ID=%d, want 2002", res.ID.ValueInt64())
	}

	// Missing both load_balancer_id and load_balancer_name
	cfgMissingLB := &LBListenerDataSourceModel{
		VpcID: types.Int64Value(10),
		Name:  types.StringValue("https-listener"),
	}
	_, diagsMissing := ds.lookup(context.Background(), cfgMissingLB)
	if !diagsMissing.HasError() {
		t.Error("expected error when neither load_balancer_id nor load_balancer_name is specified")
	}
}

func TestLBPoolDataSource_LookupByLBName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLoadBalancerList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", map[string]interface{}{
			"items": []map[string]interface{}{
				{"vttLoadBalancerId": 500, "name": "my-lb"},
			},
		}
	})
	srv.on(pathLoadBalancerPools, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "success", []map[string]interface{}{
			{
				"vttLoadBalancerPoolId": 3002,
				"vttLoadBalancerId":     500,
				"name":                  "pool-api",
				"protocol":              "HTTP",
				"algorithm":             "LEAST_CONNECTIONS",
				"status":                "ACTIVE",
			},
		}
	})

	ds := &LBPoolDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "10"}

	// Lookup using only load_balancer_name (load_balancer_id is null)
	cfgByLBName := &LBPoolDataSourceModel{
		VpcID:            types.Int64Value(10),
		LoadBalancerName: types.StringValue("my-lb"),
		Name:             types.StringValue("pool-api"),
	}
	res, diags := ds.lookup(context.Background(), cfgByLBName)
	if diags.HasError() {
		t.Fatalf("lookup by LB name failed: %v", diags)
	}
	if res.LoadBalancerID.ValueInt64() != 500 {
		t.Errorf("got LoadBalancerID=%d, want 500", res.LoadBalancerID.ValueInt64())
	}
	if res.ID.ValueInt64() != 3002 {
		t.Errorf("got ID=%d, want 3002", res.ID.ValueInt64())
	}

	// Missing both load_balancer_id and load_balancer_name
	cfgMissingLB := &LBPoolDataSourceModel{
		VpcID: types.Int64Value(10),
		Name:  types.StringValue("pool-api"),
	}
	_, diagsMissing := ds.lookup(context.Background(), cfgMissingLB)
	if !diagsMissing.HasError() {
		t.Error("expected error when neither load_balancer_id nor load_balancer_name is specified")
	}
}
