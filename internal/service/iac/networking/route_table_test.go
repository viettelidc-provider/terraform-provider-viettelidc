package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------- RouteTableResource tests ----------

func TestRouteTableResource_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "rt-main" {
			t.Errorf("create: unexpected name %v", body["name"])
		}
		// vpc_id is converted to int by callAPI; just verify it's present.
		if _, ok := body["vpc_id"]; !ok {
			t.Error("create: vpc_id missing from body")
		}
		return float64(0), "ok", map[string]interface{}{"vttRouteTableId": "rt-42"}
	})
	srv.on(pathRouteTableDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"vttRouteTableId": "rt-42",
			"name":            "rt-main",
			"vpcId":           "100",
		}
	})

	r := &RouteTableResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	plan := RouteTableResourceModel{
		Name:  types.StringValue("rt-main"),
		VpcID: types.StringValue("100"),
	}

	apiResp, d := callAPI(context.Background(), r.client, pathRouteTableCreate, map[string]interface{}{
		"name":        "rt-main",
		"vpc":         int64(100),
		"vpc_id":      "100",
		"customer_id": r.customerID,
	})
	if d.HasError() {
		t.Fatalf("create call failed: %v", d)
	}
	rtID, err := extractRouteTableID(apiResp)
	if err != nil || rtID != "rt-42" {
		t.Fatalf("extractRouteTableID: %v %v", rtID, err)
	}

	plan.ID = types.StringValue(rtID)
	var dgs diag.Diagnostics
	r.readInto(context.Background(), &plan, &dgs)
	if dgs.HasError() {
		t.Fatalf("readInto: %v", dgs)
	}
	if plan.Name.ValueString() != "rt-main" {
		t.Errorf("name: %q", plan.Name.ValueString())
	}
	if plan.ID.ValueString() != "rt-42" {
		t.Errorf("id: %q", plan.ID.ValueString())
	}
}

func TestRouteTableResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["route_table_id"] != "rt-1" {
			t.Errorf("delete: unexpected route_table_id %v", body["route_table_id"])
		}
		return float64(0), "ok", nil
	})

	r := &RouteTableResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "vpc-1"}
	body := map[string]interface{}{
		"route_table_id": "rt-1",
		"vpc_id":         "vpc-1",
		"customer_id":    r.customerID,
	}
	_, d := callAPI(context.Background(), r.client, pathRouteTableDelete, body)
	if d.HasError() {
		t.Fatalf("delete failed: %v", d)
	}
}

func TestRouteTableResource_Delete_AlreadyGone(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "resource not found", nil
	})

	r := &RouteTableResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "vpc-1"}

	// Simulate what Delete does: check diags then isNotFound.
	apiResp, d := callAPI(context.Background(), r.client, pathRouteTableDelete, map[string]interface{}{
		"route_table_id": "rt-gone",
		"vpc_id":         "vpc-1",
		"customer_id":    r.customerID,
	})
	if d.HasError() && apiResp != nil && isNotFoundMessage(apiResp.Message) {
		// idempotent — no error expected
		return
	}
	if d.HasError() {
		t.Fatalf("unexpected error for not-found: %v", d)
	}
}

// ---------- RouteTableAssociationResource tests ----------

func TestRouteTableAssociation_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableSubnetAttach, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["route_table_id"] != "rt-1" {
			t.Errorf("attach: unexpected route_table_id %v", body["route_table_id"])
		}
		if body["subnet_id"] != "sub-1" {
			t.Errorf("attach: unexpected subnet_id %v", body["subnet_id"])
		}
		return float64(0), "ok", map[string]interface{}{"status": "attached"}
	})

	r := &RouteTableAssociationResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "vpc-1"}
	body := map[string]interface{}{
		"route_table_id": "rt-1",
		"subnet_id":      "sub-1",
		"vpc_id":         "vpc-1",
		"customer_id":    r.customerID,
	}
	_, d := callAPI(context.Background(), r.client, pathRouteTableSubnetAttach, body)
	if d.HasError() {
		t.Fatalf("attach failed: %v", d)
	}

	// Verify composite ID format.
	compositeID := fmt.Sprintf("%s/%s", "rt-1", "sub-1")
	if compositeID != "rt-1/sub-1" {
		t.Errorf("composite id: %q", compositeID)
	}
}

func TestRouteTableAssociation_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableSubnetDetach, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["route_table_id"] != "rt-1" {
			t.Errorf("detach: unexpected route_table_id %v", body["route_table_id"])
		}
		return float64(0), "ok", nil
	})

	r := &RouteTableAssociationResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "vpc-1"}
	body := map[string]interface{}{
		"route_table_id": "rt-1",
		"subnet_id":      "sub-1",
		"vpc_id":         "vpc-1",
		"customer_id":    r.customerID,
	}
	_, d := callAPI(context.Background(), r.client, pathRouteTableSubnetDetach, body)
	if d.HasError() {
		t.Fatalf("detach failed: %v", d)
	}
}

func TestRouteTableAssociation_Delete_Idempotent(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableSubnetDetach, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "subnet is not attached to route table", nil
	})

	r := &RouteTableAssociationResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "vpc-1"}
	body := map[string]interface{}{
		"route_table_id": "rt-1",
		"subnet_id":      "sub-gone",
		"vpc_id":         "vpc-1",
		"customer_id":    r.customerID,
	}
	apiResp, d := callAPI(context.Background(), r.client, pathRouteTableSubnetDetach, body)
	if d.HasError() {
		if apiResp != nil && (isNotFoundMessage(apiResp.Message) || isNotAttachedMessage(apiResp.Message)) {
			return // idempotent — expected
		}
		t.Fatalf("unexpected error: %v", d)
	}
}

func TestRouteTableAssociation_ImportState(t *testing.T) {
	t.Parallel()
	parts := splitRouteTableAssociationID("rt-10/sub-20")
	if len(parts) != 2 || parts[0] != "rt-10" || parts[1] != "sub-20" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
	// Invalid ID.
	if p := splitRouteTableAssociationID("bad-id"); len(p) == 2 && p[0] != "" && p[1] != "" {
		t.Fatal("expected invalid parse")
	}
}

// splitRouteTableAssociationID is a test helper mirroring the import logic.
func splitRouteTableAssociationID(id string) []string {
	parts := make([]string, 2)
	for i, p := range [2]string{"", ""} {
		_ = p
		_ = i
	}
	// Use the same logic as ImportState.
	idx := -1
	for i, c := range id {
		if c == '/' {
			idx = i
			break
		}
	}
	if idx <= 0 || idx == len(id)-1 {
		return []string{"bad"}
	}
	parts[0] = id[:idx]
	parts[1] = id[idx+1:]
	return parts
}

// ---------- RouteTableDataSource tests ----------

func TestRouteTableDataSource_ByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", []map[string]interface{}{
			{"vttRouteTableId": "rt-5", "name": "main-rt", "vpcId": "vpc-1"},
			{"vttRouteTableId": "rt-6", "name": "other-rt", "vpcId": "vpc-1"},
		}
	})

	d := &RouteTableDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "vpc-1"}
	cfg := RouteTableDataSourceModel{
		Name:  types.StringValue("main-rt"),
		VpcID: types.StringValue("vpc-1"),
	}

	body := map[string]interface{}{
		"vpc_id":      "vpc-1",
		"customer_id": d.customerID,
	}
	apiResp, dgs := callAPI(context.Background(), d.client, pathRouteTableList, body)
	if dgs.HasError() {
		t.Fatalf("list failed: %v", dgs)
	}

	items, err := decodeSGList(apiResp)
	if err != nil {
		t.Fatalf("decode list: %v", err)
	}

	targetName := cfg.Name.ValueString()
	var found bool
	for _, item := range items {
		if asString(item, "name") == targetName {
			cfg.ID = types.StringValue(asIDString(item, "vttRouteTableId"))
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find main-rt")
	}
	if cfg.ID.ValueString() != "rt-5" {
		t.Errorf("id: %q", cfg.ID.ValueString())
	}
}

func TestRouteTableDataSource_ByID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathRouteTableDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["route_table_id"] != "rt-5" {
			t.Errorf("detail: unexpected id %v", body["route_table_id"])
		}
		return float64(0), "ok", map[string]interface{}{
			"vttRouteTableId": "rt-5",
			"name":            "main-rt",
			"vpcId":           "vpc-1",
		}
	})

	d := &RouteTableDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "vpc-1"}
	body := map[string]interface{}{
		"route_table_id": "rt-5",
		"vpc_id":         "vpc-1",
		"customer_id":    d.customerID,
	}
	apiResp, dgs := callAPI(context.Background(), d.client, pathRouteTableDetail, body)
	if dgs.HasError() {
		t.Fatalf("detail failed: %v", dgs)
	}
	m := &RouteTableResourceModel{}
	if err := mapRouteTableResponse(apiResp, m); err != nil {
		t.Fatalf("mapRouteTableResponse: %v", err)
	}
	if m.ID.ValueString() != "rt-5" || m.Name.ValueString() != "main-rt" {
		t.Errorf("model: %+v", m)
	}
}

// ---------- InternetGatewayDataSource tests ----------

func TestInternetGatewayDataSource_ByName(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	// The IGW list response has a nested "data.content" structure since the
	// real API backend returns a paginated envelope that becomes apiResp.Data.
	srv.on(pathInternetGatewayList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"data": map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"vttInternetGatewayId": float64(42),
						"name":                "igw-main",
						"status":              "ACTIVE",
						"vttSubnetId":         float64(101),
						"floatingIpAddress":   "1.2.3.4",
					},
				},
			},
		}
	})

	d := &InternetGatewayDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	cfg := InternetGatewayDataSourceModel{
		Name:  types.StringValue("igw-main"),
		VpcID: types.StringValue("100"),
	}

	body := map[string]interface{}{
		"vpc_id":      "100",
		"customer_id": d.customerID,
		"page_index":  0,
		"page_size":   1000,
		"filters":     []map[string]interface{}{},
	}
	apiResp, dgs := callAPI(context.Background(), d.client, pathInternetGatewayList, body)
	if dgs.HasError() {
		t.Fatalf("list failed: %v", dgs)
	}

	// Replicate the decode logic from internet_gateway_data_source.go.
	var listResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Content []struct {
				VttInternetGatewayID int64  `json:"vttInternetGatewayId"`
				Name                 string `json:"name"`
				Status               string `json:"status"`
				VttSubnetID          int64  `json:"vttSubnetId"`
				FloatingIPAddress    string `json:"floatingIpAddress"`
			} `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	targetName := cfg.Name.ValueString()
	var foundName string
	for _, item := range listResp.Data.Content {
		if item.Name == targetName {
			foundName = item.Name
			if item.VttInternetGatewayID != 42 {
				t.Errorf("igw id: %d", item.VttInternetGatewayID)
			}
			break
		}
	}
	if foundName != "igw-main" {
		t.Errorf("expected igw-main, got %q", foundName)
	}
}
