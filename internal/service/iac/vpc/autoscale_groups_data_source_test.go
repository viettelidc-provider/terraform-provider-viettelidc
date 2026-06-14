package vpc

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------- AutoscaleGroupsDataSource tests (Story 9.5) ----------

func TestUnit_AutoscaleGroupsDataSource_All(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{
			{
				"id": float64(10), "launchTemplateId": float64(1),
				"isAutoscale": true, "desiredCapacity": float64(2), "minSize": float64(1), "maxSize": float64(5),
				"metricType": "CPU", "scaleOutThreshold": float64(80), "scaleInThreshold": float64(20),
				"hasLoadBalancer": false,
			},
			{
				"id": float64(20), "launchTemplateId": float64(2),
				"isAutoscale": false, "desiredCapacity": float64(1), "minSize": float64(1), "maxSize": float64(3),
				"metricType": "CPU", "scaleOutThreshold": float64(70), "scaleInThreshold": float64(30),
				"hasLoadBalancer": true,
			},
		}
	})

	r := &AutoscaleGroupsDataSource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	body := map[string]interface{}{
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(context.Background(), r.client, pathAutoscaleGroupList, body)
	if diags.HasError() {
		t.Fatalf("list call failed: %v", diags)
	}

	items, err := decodeAutoscaleGroupList(apiResp)
	if err != nil {
		t.Fatalf("decodeAutoscaleGroupList: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if asIDString(items[0], "id") != "10" {
		t.Errorf("item[0] id: %v", items[0]["id"])
	}
	if asIDString(items[1], "id") != "20" {
		t.Errorf("item[1] id: %v", items[1]["id"])
	}
}

func TestUnit_AutoscaleGroupsDataSource_Empty(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{}
	})

	r := &AutoscaleGroupsDataSource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	body := map[string]interface{}{
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(context.Background(), r.client, pathAutoscaleGroupList, body)
	if diags.HasError() {
		t.Fatalf("list call failed: %v", diags)
	}

	items, err := decodeAutoscaleGroupList(apiResp)
	if err != nil {
		t.Fatalf("decodeAutoscaleGroupList: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestUnit_AutoscaleGroupsDataSource_Int64Fields(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{
			{
				"id": float64(1), "launchTemplateId": float64(99),
				"isAutoscale": true, "desiredCapacity": float64(4), "minSize": float64(2), "maxSize": float64(8),
				"metricType": "CPU", "scaleOutThreshold": float64(85), "scaleInThreshold": float64(15),
				"hasLoadBalancer": false,
			},
		}
	})

	r := &AutoscaleGroupsDataSource{client: srv.newClient(), customerID: "c", defaultVpcID: "v"}

	body := map[string]interface{}{"vpc_id": "v", "customer_id": "c"}
	apiResp, _ := callAPI(context.Background(), r.client, pathAutoscaleGroupList, body)
	items, _ := decodeAutoscaleGroupList(apiResp)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	dc := asInt64(items[0], "desiredCapacity")
	min := asInt64(items[0], "minSize")
	max := asInt64(items[0], "maxSize")
	sot := asInt64(items[0], "scaleOutThreshold")
	sit := asInt64(items[0], "scaleInThreshold")

	if dc != 4 {
		t.Errorf("desiredCapacity: %d", dc)
	}
	if min != 2 {
		t.Errorf("minSize: %d", min)
	}
	if max != 8 {
		t.Errorf("maxSize: %d", max)
	}
	if sot != 85 {
		t.Errorf("scaleOutThreshold: %d", sot)
	}
	if sit != 15 {
		t.Errorf("scaleInThreshold: %d", sit)
	}
}

func TestUnit_AutoscaleGroupsDataSource_DefaultVpcID(t *testing.T) {
	t.Parallel()
	r := &AutoscaleGroupsDataSource{client: nil, customerID: "c", defaultVpcID: "default-vpc"}

	got, diags := resolveVpcID("", r.defaultVpcID)
	if diags.HasError() {
		t.Fatalf("unexpected diag: %v", diags)
	}
	if got != "default-vpc" {
		t.Errorf("got %q want default-vpc", got)
	}
}

func TestUnit_AutoscaleGroupsDataSource_BoolFields(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{
			{
				"id": float64(1), "launchTemplateId": float64(5),
				"isAutoscale": true, "desiredCapacity": float64(1), "minSize": float64(1), "maxSize": float64(2),
				"metricType": "CPU", "scaleOutThreshold": float64(90), "scaleInThreshold": float64(10),
				"hasLoadBalancer": true,
			},
		}
	})

	r := &AutoscaleGroupsDataSource{client: srv.newClient(), customerID: "c", defaultVpcID: "v"}

	body := map[string]interface{}{"vpc_id": "v", "customer_id": "c"}
	apiResp, _ := callAPI(context.Background(), r.client, pathAutoscaleGroupList, body)
	items, _ := decodeAutoscaleGroupList(apiResp)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	isAuto := asBool(items[0], "isAutoscale")
	hasLB := asBool(items[0], "hasLoadBalancer")

	if !isAuto {
		t.Errorf("isAutoscale: expected true, got false")
	}
	if !hasLB {
		t.Errorf("hasLoadBalancer: expected true, got false")
	}
}

func TestUnit_AutoscaleGroupsDataSource_Schema(t *testing.T) {
	t.Parallel()
	r := &AutoscaleGroupsDataSource{}
	var sreq datasource.SchemaRequest
	var sresp datasource.SchemaResponse
	r.Schema(context.Background(), sreq, &sresp)

	if _, ok := sresp.Schema.Attributes["vpc_id"]; !ok {
		t.Error("vpc_id attribute missing from ASG data source schema")
	}
	if _, ok := sresp.Schema.Attributes["autoscale_groups"]; !ok {
		t.Error("autoscale_groups attribute missing from ASG data source schema")
	}
}

// ---------- AutoscaleGroupItem model tests ----------

func TestUnit_AutoscaleGroupItem_Mapping(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"id": float64(55), "launchTemplateId": float64(7),
		"isAutoscale": true, "desiredCapacity": float64(2),
		"minSize": float64(1), "maxSize": float64(5),
		"metricType": "CPU", "scaleOutThreshold": float64(80),
		"scaleInThreshold": float64(20), "hasLoadBalancer": false,
	}

	item := AutoscaleGroupItem{
		ID:                types.StringValue(asIDString(raw, "id")),
		LaunchTemplateID:  types.StringValue(asIDString(raw, "launchTemplateId")),
		IsAutoscale:       types.BoolValue(asBool(raw, "isAutoscale")),
		DesiredCapacity:   types.Int64Value(asInt64(raw, "desiredCapacity")),
		MinSize:           types.Int64Value(asInt64(raw, "minSize")),
		MaxSize:           types.Int64Value(asInt64(raw, "maxSize")),
		MetricType:        types.StringValue(asString(raw, "metricType")),
		ScaleOutThreshold: types.Int64Value(asInt64(raw, "scaleOutThreshold")),
		ScaleInThreshold:  types.Int64Value(asInt64(raw, "scaleInThreshold")),
		HasLoadBalancer:   types.BoolValue(asBool(raw, "hasLoadBalancer")),
	}

	if item.ID.ValueString() != "55" {
		t.Errorf("ID: %s", item.ID.ValueString())
	}
	if !item.IsAutoscale.ValueBool() {
		t.Error("IsAutoscale should be true")
	}
	if item.DesiredCapacity.ValueInt64() != 2 {
		t.Errorf("DesiredCapacity: %d", item.DesiredCapacity.ValueInt64())
	}
}
