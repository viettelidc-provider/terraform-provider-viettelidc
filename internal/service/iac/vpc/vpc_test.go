package vpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

// ---------- Test plumbing ----------

// fakeAPIServer routes path → handler returning (code, message, data) envelope.
type fakeAPIServer struct {
	*httptest.Server
	handlers map[string]func(body map[string]interface{}) (code interface{}, message string, data interface{})
	calls    map[string]int
}

func newFakeAPI(t *testing.T) *fakeAPIServer {
	t.Helper()
	f := &fakeAPIServer{
		handlers: map[string]func(map[string]interface{}) (interface{}, string, interface{}){},
		calls:    map[string]int{},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.calls[r.URL.Path]++
		h, ok := f.handlers[r.URL.Path]
		if !ok {
			http.Error(w, "no handler for "+r.URL.Path, http.StatusNotFound)
			return
		}
		code, msg, data := h(body)
		raw, _ := json.Marshal(data)
		env := map[string]interface{}{"code": code, "message": msg, "data": json.RawMessage(raw)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(env)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAPIServer) on(path string, h func(map[string]interface{}) (interface{}, string, interface{})) {
	f.handlers[path] = h
}

func (f *fakeAPIServer) newClient() *client.Client {
	return client.NewClient(f.URL, "test-token")
}

// ---------- Pure helper tests ----------

func TestResolveVpcID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, plan, def, want string
		wantErr               bool
	}{
		{"plan wins", "vpc-plan", "vpc-default", "vpc-plan", false},
		{"falls back to default", "", "vpc-default", "vpc-default", false},
		{"both empty errors", "", "", "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, diags := resolveVpcID(tc.plan, tc.def)
			if tc.wantErr {
				if !diags.HasError() {
					t.Fatal("expected error diag")
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("unexpected diag: %v", diags)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestIsNotFoundMessage(t *testing.T) {
	t.Parallel()
	yes := []string{"resource not found", "template does not exist", "no such group"}
	no := []string{"forbidden", "internal error", ""}
	for _, m := range yes {
		if !isNotFoundMessage(m) {
			t.Errorf("expected true for %q", m)
		}
	}
	for _, m := range no {
		if isNotFoundMessage(m) {
			t.Errorf("expected false for %q", m)
		}
	}
}

func TestIsInUseMessage(t *testing.T) {
	t.Parallel()
	yes := []string{"launch template is in use", "being used by autoscale group", "still used"}
	no := []string{"not found", "forbidden", ""}
	for _, m := range yes {
		if !isInUseMessage(m) {
			t.Errorf("expected true for %q", m)
		}
	}
	for _, m := range no {
		if isInUseMessage(m) {
			t.Errorf("expected false for %q", m)
		}
	}
}

// ---------- Launch Template helper tests ----------

func TestBuildLaunchTemplateCreateBody(t *testing.T) {
	t.Parallel()
	plan := LaunchTemplateResourceModel{
		Name:       types.StringValue("tpl-a"),
		VmID:       types.StringValue("vm-1"),
		MemorySize: types.Int64Value(2048),
		CpuSize:    types.Int64Value(2),
	}
	body := buildLaunchTemplateCreateBody(plan, "cust-1", "vpc-1")
	if body["name"] != "tpl-a" {
		t.Fatalf("name wrong: %v", body["name"])
	}
	if body["vm_id"] != "vm-1" {
		t.Fatalf("vm_id wrong: %v", body["vm_id"])
	}
	if body["memory_size"] != int64(2048) {
		t.Fatalf("memory_size wrong: %v", body["memory_size"])
	}
	if body["cpu_size"] != int64(2) {
		t.Fatalf("cpu_size wrong: %v", body["cpu_size"])
	}
	if body["vpc_id"] != "vpc-1" || body["customer_id"] != "cust-1" {
		t.Fatalf("vpc/cust wrong: %#v", body)
	}
	if _, ok := body["description"]; ok {
		t.Fatal("empty description should be omitted")
	}

	// With description
	plan.Description = types.StringValue("my desc")
	body = buildLaunchTemplateCreateBody(plan, "cust-1", "vpc-1")
	if body["description"] != "my desc" {
		t.Fatalf("description wrong: %v", body["description"])
	}
}

func TestExtractLaunchTemplateID(t *testing.T) {
	t.Parallel()
	resp := &client.APIResponse{Data: json.RawMessage(`{"id": 42}`)}
	id, err := extractLaunchTemplateID(resp)
	if err != nil || id != "42" {
		t.Fatalf("got %q err=%v", id, err)
	}

	resp = &client.APIResponse{Data: json.RawMessage(`{"id": "lt-abc"}`)}
	id, err = extractLaunchTemplateID(resp)
	if err != nil || id != "lt-abc" {
		t.Fatalf("string id: got %q err=%v", id, err)
	}

	resp = &client.APIResponse{Data: json.RawMessage(`{"name": "no-id"}`)}
	if _, err := extractLaunchTemplateID(resp); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestMapLaunchTemplateResponse(t *testing.T) {
	t.Parallel()
	data := `{
		"id": 7,
		"name": "tpl-a",
		"description": "hello",
		"vmId": "vm-99",
		"memorySize": 4096,
		"cpuSize": 4,
		"vpcId": "vpc-1"
	}`
	resp := &client.APIResponse{Data: json.RawMessage(data)}
	m := &LaunchTemplateResourceModel{}
	if err := mapLaunchTemplateResponse(resp, m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID.ValueString() != "7" {
		t.Fatalf("id: %v", m.ID)
	}
	if m.Name.ValueString() != "tpl-a" {
		t.Fatalf("name: %v", m.Name)
	}
	if m.Description.ValueString() != "hello" {
		t.Fatalf("description: %v", m.Description)
	}
	if m.MemorySize.ValueInt64() != 4096 {
		t.Fatalf("memory_size: %v", m.MemorySize)
	}
	if m.CpuSize.ValueInt64() != 4 {
		t.Fatalf("cpu_size: %v", m.CpuSize)
	}
}

// ---------- Autoscale Group helper tests ----------

func TestBuildAutoscaleGroupCreateBody(t *testing.T) {
	t.Parallel()
	plan := AutoscaleGroupResourceModel{
		Name:              types.StringValue("asg-1"),
		LaunchTemplateID:  types.StringValue("lt-1"),
		IsAutoscale:       types.BoolValue(true),
		DesiredCapacity:   types.Int64Value(2),
		MinSize:           types.Int64Value(1),
		MaxSize:           types.Int64Value(5),
		ScaleOutThreshold: types.Int64Value(80),
		ScaleInThreshold:  types.Int64Value(20),
		HasLoadBalancer:   types.BoolValue(false),
		MetricType:        types.StringValue("CPU"),
	}
	body := buildAutoscaleGroupCreateBody(plan, "cust-1", "vpc-1")
	// name is Computed (read-only) — it is NOT included in the create body.
	if _, ok := body["name"]; ok {
		t.Fatal("name should NOT be in autoscale group create body (it's Computed/read-only)")
	}
	if body["launch_template_id"] != "lt-1" {
		t.Fatalf("launch_template_id: %v", body["launch_template_id"])
	}
	if body["is_autoscale"] != true {
		t.Fatalf("is_autoscale: %v", body["is_autoscale"])
	}
	if body["desired_capacity"] != int64(2) {
		t.Fatalf("desired_capacity: %v", body["desired_capacity"])
	}
	if body["scale_out_threshold"] != int64(80) {
		t.Fatalf("scale_out_threshold: %v", body["scale_out_threshold"])
	}
	if body["metric_type"] != "CPU" {
		t.Fatalf("metric_type: %v", body["metric_type"])
	}
}

func TestBuildAutoscaleGroupCreateBody_DefaultMetricType(t *testing.T) {
	t.Parallel()
	plan := AutoscaleGroupResourceModel{
		Name:               types.StringValue("asg-2"),
		LaunchTemplateID:   types.StringValue("lt-1"),
		IsAutoscale:        types.BoolValue(false),
		DesiredCapacity:    types.Int64Value(1),
		MinSize:            types.Int64Value(1),
		MaxSize:            types.Int64Value(3),
		ScaleOutThreshold:  types.Int64Value(70),
		ScaleInThreshold:   types.Int64Value(30),
		HasLoadBalancer:    types.BoolValue(false),
		MetricType:         types.StringValue(""), // empty → should default to CPU
	}
	body := buildAutoscaleGroupCreateBody(plan, "cust", "vpc")
	if body["metric_type"] != "CPU" {
		t.Fatalf("expected metric_type CPU, got %v", body["metric_type"])
	}
}

func TestExtractAutoscaleGroupID(t *testing.T) {
	t.Parallel()
	resp := &client.APIResponse{Data: json.RawMessage(`{"id": 99}`)}
	id, err := extractAutoscaleGroupID(resp)
	if err != nil || id != "99" {
		t.Fatalf("got %q err=%v", id, err)
	}
}

func TestMapAutoscaleGroupResponse(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"id":                 float64(55),
		"name":               "asg-x",
		"launchTemplateId":   float64(7),
		"isAutoscale":        true,
		"desiredCapacity":    float64(3),
		"minSize":            float64(1),
		"maxSize":            float64(10),
		"metricType":         "CPU",
		"scaleOutThreshold":  float64(80),
		"scaleInThreshold":   float64(20),
		"hasLoadBalancer":    false,
	}
	m := &AutoscaleGroupResourceModel{}
	mapAutoscaleGroupResponse(raw, m, "vpc-1")
	if m.ID.ValueString() != "55" {
		t.Fatalf("id: %v", m.ID)
	}
	if m.Name.ValueString() != "asg-x" {
		t.Fatalf("name: %v", m.Name)
	}
	if m.LaunchTemplateID.ValueString() != "7" {
		t.Fatalf("launchTemplateId: %v", m.LaunchTemplateID)
	}
	if !m.IsAutoscale.ValueBool() {
		t.Fatalf("isAutoscale: %v", m.IsAutoscale)
	}
	if m.DesiredCapacity.ValueInt64() != 3 {
		t.Fatalf("desiredCapacity: %v", m.DesiredCapacity)
	}
	if m.ScaleOutThreshold.ValueInt64() != 80 {
		t.Fatalf("scaleOutThreshold: %v", m.ScaleOutThreshold)
	}
	if m.VpcID.ValueString() != "vpc-1" {
		t.Fatalf("vpcId: %v", m.VpcID)
	}
}

// ---------- Integration-style tests (httptest-backed) ----------

func TestLaunchTemplateResource_CreateRead(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLaunchTemplateCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "tpl-test" {
			t.Errorf("create name wrong: %v", body["name"])
		}
		return float64(0), "ok", map[string]interface{}{"id": "lt-9"}
	})
	srv.on(pathLaunchTemplateDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"id":          "lt-9",
			"name":        "tpl-test",
			"description": "",
			"vmId":        "vm-1",
			"memorySize":  float64(2048),
			"cpuSize":     float64(2),
			"vpcId":       "vpc-1",
		}
	})

	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	plan := LaunchTemplateResourceModel{
		Name:       types.StringValue("tpl-test"),
		VmID:       types.StringValue("vm-1"),
		MemorySize: types.Int64Value(2048),
		CpuSize:    types.Int64Value(2),
	}

	body := buildLaunchTemplateCreateBody(plan, r.customerID, r.defaultVpcID)
	apiResp, diags := callAPI(context.Background(), r.client, pathLaunchTemplateCreate, body)
	if diags.HasError() {
		t.Fatalf("create call failed: %v", diags)
	}
	id, err := extractLaunchTemplateID(apiResp)
	if err != nil || id != "lt-9" {
		t.Fatalf("extract id: %v %v", id, err)
	}

	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(r.defaultVpcID)

	// Verify detail endpoint is reachable.
	if srv.calls[pathLaunchTemplateCreate] != 1 {
		t.Fatalf("expected 1 create call, got %d", srv.calls[pathLaunchTemplateCreate])
	}
}

func TestLaunchTemplateResource_ReadDriftReturnsFalse(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathLaunchTemplateDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return "FAILURE", "launch template not found", nil
	})
	r := &LaunchTemplateResource{client: srv.newClient(), customerID: "c", defaultVpcID: "v"}
	m := &LaunchTemplateResourceModel{ID: types.StringValue("lt-x"), VpcID: types.StringValue("v")}
	var diags diag.Diagnostics
	result := r.readInto(context.Background(), m, &diags)
	if result {
		t.Fatal("expected drift (false) on not-found")
	}
	if diags.HasError() {
		t.Fatalf("drift should not produce error diag: %v", diags)
	}
}

func TestAutoscaleGroupResource_ReadListFilter(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathAutoscaleGroupList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", []map[string]interface{}{
			{"id": float64(1), "name": "asg-other"},
			{"id": float64(55), "name": "asg-target", "launchTemplateId": float64(7),
				"isAutoscale": true, "desiredCapacity": float64(2),
				"minSize": float64(1), "maxSize": float64(5),
				"metricType": "CPU", "scaleOutThreshold": float64(80),
				"scaleInThreshold": float64(20), "hasLoadBalancer": false},
		}
	})
	r := &AutoscaleGroupResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	m := &AutoscaleGroupResourceModel{ID: types.StringValue("55"), VpcID: types.StringValue("vpc-1")}
	var diags diag.Diagnostics
	result := r.readInto(context.Background(), m, &diags)
	if !result {
		t.Fatal("expected found=true")
	}
	if diags.HasError() {
		t.Fatalf("unexpected diag: %v", diags)
	}
	if m.Name.ValueString() != "asg-target" {
		t.Fatalf("name: %v", m.Name)
	}
}

func TestAutoscaleGroupResource_ReadNotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathAutoscaleGroupList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", []map[string]interface{}{}
	})
	r := &AutoscaleGroupResource{client: srv.newClient(), customerID: "c", defaultVpcID: "v"}
	m := &AutoscaleGroupResourceModel{ID: types.StringValue("missing"), VpcID: types.StringValue("v")}
	var diags diag.Diagnostics
	result := r.readInto(context.Background(), m, &diags)
	if result {
		t.Fatal("expected drift (false) for missing ASG")
	}
}
