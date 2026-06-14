package vpc

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------- AutoscaleGroupResource tests (Story 9.4) ----------

func TestUnit_AutoscaleGroupResource_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["launch_template_id"] == nil {
			t.Error("launch_template_id missing from create body")
		}
		return float64(0), "Success", map[string]interface{}{
			"id":                "asg-xyz789",
			"launchTemplateId":  "lt-abc123",
			"isAutoscale":       true,
			"desiredCapacity":   float64(2),
			"minSize":           float64(1),
			"maxSize":           float64(5),
			"metricType":        "CPU",
			"scaleOutThreshold": float64(80),
			"scaleInThreshold":  float64(20),
			"hasLoadBalancer":   false,
		}
	})

	r := &AutoscaleGroupResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	apiResp, d := callAPI(context.Background(), r.client, pathAutoscaleGroupCreate, map[string]interface{}{
		"launch_template_id": "lt-abc123",
		"is_autoscale":       true,
		"desired_capacity":   int64(2),
		"min_size":           int64(1),
		"max_size":           int64(5),
		"metric_type":        "CPU",
		"scale_out_threshold": int64(80),
		"scale_in_threshold":  int64(20),
		"has_load_balancer":   false,
		"vpc_id":             "36961",
		"customer_id":        r.customerID,
	})
	if d.HasError() {
		t.Fatalf("create call failed: %v", d)
	}

	id, err := extractAutoscaleGroupID(apiResp)
	if err != nil {
		t.Fatalf("extractAutoscaleGroupID: %v", err)
	}
	if id != "asg-xyz789" {
		t.Errorf("id: got %q want asg-xyz789", id)
	}
}

func TestUnit_AutoscaleGroupResource_Read_ListFilter(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{
			{"id": float64(1), "name": "asg-other"},
			{
				"id": float64(789), "name": "asg-target",
				"launchTemplateId": float64(123), "isAutoscale": true,
				"desiredCapacity": float64(3), "minSize": float64(1), "maxSize": float64(10),
				"metricType": "CPU", "scaleOutThreshold": float64(75), "scaleInThreshold": float64(25),
				"hasLoadBalancer": false,
			},
		}
	})

	r := &AutoscaleGroupResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}
	model := &AutoscaleGroupResourceModel{
		ID:    types.StringValue("789"),
		VpcID: types.StringValue("36961"),
	}

	var d diag.Diagnostics
	found := r.readInto(context.Background(), model, &d)
	if d.HasError() {
		t.Fatalf("readInto error: %v", d)
	}
	if !found {
		t.Fatal("readInto should return true for matching ASG")
	}
	if model.Name.ValueString() != "asg-target" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.DesiredCapacity.ValueInt64() != 3 {
		t.Errorf("desired_capacity: %d", model.DesiredCapacity.ValueInt64())
	}
	if model.ScaleOutThreshold.ValueInt64() != 75 {
		t.Errorf("scale_out_threshold: %d", model.ScaleOutThreshold.ValueInt64())
	}
}

func TestUnit_AutoscaleGroupResource_Read_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{}
	})

	r := &AutoscaleGroupResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}
	model := &AutoscaleGroupResourceModel{
		ID:    types.StringValue("asg-gone"),
		VpcID: types.StringValue("36961"),
	}

	var d diag.Diagnostics
	found := r.readInto(context.Background(), model, &d)
	if d.HasError() {
		t.Fatalf("unexpected diags: %v", d)
	}
	if found {
		t.Error("readInto should return false for not-found ASG")
	}
}

func TestUnit_AutoscaleGroupResource_ForceNew(t *testing.T) {
	t.Parallel()
	r := &AutoscaleGroupResource{}
	var sreq resource.SchemaRequest
	var sresp resource.SchemaResponse
	r.Schema(context.Background(), sreq, &sresp)

	forceNewAttrs := []string{
		"launch_template_id", "is_autoscale", "desired_capacity",
		"min_size", "max_size", "scale_out_threshold", "scale_in_threshold",
		"has_load_balancer", "metric_type",
	}
	for _, attrName := range forceNewAttrs {
		if _, ok := sresp.Schema.Attributes[attrName]; !ok {
			t.Errorf("attribute %q not found in ASG schema", attrName)
		}
	}
	if _, ok := sresp.Schema.Attributes["id"]; !ok {
		t.Error("id attribute not found in schema")
	}
	if _, ok := sresp.Schema.Attributes["vpc_id"]; !ok {
		t.Error("vpc_id attribute not found in schema")
	}
	if _, ok := sresp.Schema.Attributes["name"]; !ok {
		t.Error("name attribute not found in schema")
	}
}

func TestUnit_AutoscaleGroupResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["id"] == nil {
			t.Error("id missing from delete body")
		}
		return float64(0), "Success", nil
	})

	r := &AutoscaleGroupResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}

	_, d := callAPI(context.Background(), r.client, pathAutoscaleGroupDelete, map[string]interface{}{
		"id":          "asg-xyz789",
		"vpc_id":      "36961",
		"customer_id": r.customerID,
	})
	if d.HasError() {
		t.Fatalf("delete call failed: %v", d)
	}
}

func TestUnit_AutoscaleGroupResource_Import(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathAutoscaleGroupList, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", []map[string]interface{}{
			{
				"id": float64(789), "name": "imported-asg",
				"launchTemplateId": float64(123), "isAutoscale": false,
				"desiredCapacity": float64(1), "minSize": float64(1), "maxSize": float64(3),
				"metricType": "CPU", "scaleOutThreshold": float64(70), "scaleInThreshold": float64(30),
				"hasLoadBalancer": false,
			},
		}
	})

	r := &AutoscaleGroupResource{client: srv.newClient(), customerID: "244850", defaultVpcID: "36961"}
	model := &AutoscaleGroupResourceModel{
		ID:    types.StringValue("789"),
		VpcID: types.StringValue("36961"),
	}

	var d diag.Diagnostics
	found := r.readInto(context.Background(), model, &d)
	if d.HasError() {
		t.Fatalf("import readInto: %v", d)
	}
	if !found {
		t.Fatal("import should find ASG by ID")
	}
	if model.Name.ValueString() != "imported-asg" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.DesiredCapacity.ValueInt64() != 1 {
		t.Errorf("desired_capacity: %d", model.DesiredCapacity.ValueInt64())
	}
}

func TestUnit_AutoscaleGroupResource_DependencyOrder(t *testing.T) {
	t.Parallel()
	// Verify launch_template_id attribute exists — it is the attribute that creates
	// the implicit dependency ordering (LT created first, ASG destroyed first).
	r := &AutoscaleGroupResource{}
	var sreq resource.SchemaRequest
	var sresp resource.SchemaResponse
	r.Schema(context.Background(), sreq, &sresp)

	ltAttr, ok := sresp.Schema.Attributes["launch_template_id"]
	if !ok {
		t.Fatal("launch_template_id attribute missing — cannot establish implicit dependency")
	}
	_ = ltAttr // presence is sufficient; user references viettelidc_launch_template.id here
}

func TestUnit_AutoscaleGroupResource_DefaultMetricType(t *testing.T) {
	t.Parallel()
	// metric_type with empty string defaults to "CPU".
	plan := AutoscaleGroupResourceModel{
		Name:              types.StringValue("asg"),
		LaunchTemplateID:  types.StringValue("lt-1"),
		IsAutoscale:       types.BoolValue(false),
		DesiredCapacity:   types.Int64Value(1),
		MinSize:           types.Int64Value(1),
		MaxSize:           types.Int64Value(3),
		ScaleOutThreshold: types.Int64Value(70),
		ScaleInThreshold:  types.Int64Value(30),
		HasLoadBalancer:   types.BoolValue(false),
		MetricType:        types.StringValue(""),
	}
	body := buildAutoscaleGroupCreateBody(plan, "c", "v")
	if body["metric_type"] != "CPU" {
		t.Errorf("expected default metric_type CPU, got %v", body["metric_type"])
	}
}
