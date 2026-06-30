// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

const pathSubnetGroupDetail = "/csa/api/v1/dbaas/subnet-group/detail"

// ---------- Test plumbing ----------

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

		key := r.Method + ":" + r.URL.Path
		f.calls[key]++
		f.calls[r.URL.Path]++

		h, ok := f.handlers[key]
		if !ok {
			h, ok = f.handlers[r.URL.Path]
		}
		if !ok {
			http.Error(w, "no handler for "+key, http.StatusNotFound)
			return
		}
		code, msg, data := h(body)
		if code == "RAW" {
			w.Header().Set("Content-Type", "text/plain")
			if s, ok := data.(string); ok {
				_, _ = w.Write([]byte(s))
			} else {
				raw, _ := json.Marshal(data)
				_, _ = w.Write(raw)
			}
			return
		}
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

// ---------- Common helper tests ----------

func TestUnit_IsNotFoundMessage(t *testing.T) {
	t.Parallel()
	yes := []string{
		"resource not found", "ERROR_NOT_FOUND", "subnet does not exist", "no such resource",
	}
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

func TestUnit_ResolveVpcID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, plan, def, want string
		wantErr               bool
	}{
		{"plan wins", "plan-vpc", "default-vpc", "plan-vpc", false},
		{"fallback to default", "", "default-vpc", "default-vpc", false},
		{"both empty errors", "", "", "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, d := resolveVpcID(tc.plan, tc.def)
			if tc.wantErr {
				if !d.HasError() {
					t.Fatal("expected error diag")
				}
				return
			}
			if d.HasError() {
				t.Fatalf("unexpected diag: %v", d)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// ---------- Story 11.2: VDBSDatabaseInstanceResource tests ----------

func TestUnit_DBInstance_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBInstanceCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "prod-db" {
			t.Errorf("create: unexpected name %v", body["name"])
		}
		if body["flavor_id"] != "flavor-medium" {
			t.Errorf("create: unexpected flavor_id %v", body["flavor_id"])
		}
		return float64(0), "Success", map[string]interface{}{"id": "dbs-abc123"}
	})

	r := &VDBSDatabaseInstanceResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}

	createBody := map[string]interface{}{
		"name":                 "prod-db",
		"flavor_id":            "flavor-medium",
		"volume_size":          float64(100),
		"db_subnet_group_name": "db-subnet-group",
		"admin_password":       "secret",
		"vpc_id":               "100",
		"customer_id":          "cust-1",
	}
	apiResp, d := callAPI(context.Background(), r.client, pathDBInstanceCreate, createBody)
	if d.HasError() {
		t.Fatalf("create call failed: %v", d)
	}

	id := extractIDFromData(apiResp)
	if id != "dbs-abc123" {
		t.Errorf("expected id dbs-abc123, got %q", id)
	}
}

func TestUnit_DBInstance_PollUntilActive(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	callCount := 0
	srv.on(pathDBInstanceDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		callCount++
		status := "BUILDING"
		if callCount >= 2 {
			status = "ACTIVE"
		}
		return float64(0), "ok", map[string]interface{}{"id": "dbs-1", "status": status}
	})

	r := &VDBSDatabaseInstanceResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	err := r.pollUntilDBActive(context.Background(), "dbs-1", "100", 5*time.Second, false)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 poll calls, got %d", callCount)
	}
}

func TestUnit_DBInstance_PollErrorState(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBInstanceDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{"id": "dbs-1", "status": "ERROR"}
	})

	r := &VDBSDatabaseInstanceResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	err := r.pollUntilDBActive(context.Background(), "dbs-1", "100", 30*time.Second, false)
	if err == nil {
		t.Fatal("expected error for ERROR status")
	}
}

func TestUnit_DBInstance_PollDeleteMode_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBInstanceDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "ERROR_NOT_FOUND", nil
	})

	r := &VDBSDatabaseInstanceResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	err := r.pollUntilDBActive(context.Background(), "dbs-1", "100", 30*time.Second, true)
	if err != nil {
		t.Errorf("expected nil (delete success), got: %v", err)
	}
}

func TestUnit_DBInstance_ReadAndMerge(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBInstanceDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", map[string]interface{}{
			"id":                 "dbs-abc",
			"name":               "prod-db",
			"status":             "ACTIVE",
			"flavorId":           "flavor-medium",
			"volumeSize":         float64(100),
			"dbSubnetGroupName":  "sg-name",
			"parameterGroupName": "pg-name",
		}
	})

	r := &VDBSDatabaseInstanceResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	model := &VDBSDatabaseInstanceResourceModel{
		ID:    types.StringValue("dbs-abc"),
		VpcID: types.StringValue("100"),
	}

	var diagsVal diag.Diagnostics
	found := r.readAndMerge(context.Background(), model, &diagsVal)
	if !found {
		t.Fatal("expected found=true")
	}
	if diagsVal.HasError() {
		t.Fatalf("unexpected diag error: %v", diagsVal)
	}
	if model.Name.ValueString() != "prod-db" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.Status.ValueString() != "ACTIVE" {
		t.Errorf("status: %q", model.Status.ValueString())
	}
	if model.FlavorID.ValueString() != "flavor-medium" {
		t.Errorf("flavor_id: %q", model.FlavorID.ValueString())
	}
	if model.VolumeSize.ValueInt64() != 100 {
		t.Errorf("volume_size: %d", model.VolumeSize.ValueInt64())
	}
}

func TestUnit_DBInstance_ReadAndMerge_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBInstanceDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(-1), "ERROR_NOT_FOUND", nil
	})

	r := &VDBSDatabaseInstanceResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	model := &VDBSDatabaseInstanceResourceModel{
		ID:    types.StringValue("dbs-missing"),
		VpcID: types.StringValue("100"),
	}

	var diagsVal diag.Diagnostics
	found := r.readAndMerge(context.Background(), model, &diagsVal)
	if found {
		t.Fatal("expected found=false for not-found response")
	}
	if diagsVal.HasError() {
		t.Fatalf("unexpected diag error on not-found: %v", diagsVal)
	}
}

// ---------- Story 11.4: VDBSParameterGroupResource tests ----------

func TestUnit_ParamGroup_BuildParameters_Single(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	paramObjType := types.ObjectType{AttrTypes: parameterAttrTypes}
	params := []ParameterModel{
		{Name: types.StringValue("max_connections"), Value: types.StringValue("1000")},
	}
	list, diags := types.ListValueFrom(ctx, paramObjType, params)
	if diags.HasError() {
		t.Fatalf("build list: %v", diags)
	}

	var diagsVal diag.Diagnostics
	entries := buildParameterEntries(ctx, list, &diagsVal)
	if diagsVal.HasError() {
		t.Fatalf("build entries error: %v", diagsVal)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "max_connections" || entries[0].Value != "1000" {
		t.Errorf("entry[0]: %+v", entries[0])
	}
}

func TestUnit_ParamGroup_BuildParameters_Multiple(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	paramObjType := types.ObjectType{AttrTypes: parameterAttrTypes}
	params := []ParameterModel{
		{Name: types.StringValue("max_connections"), Value: types.StringValue("1000")},
		{Name: types.StringValue("innodb_buffer_pool_size"), Value: types.StringValue("2G")},
	}
	list, diags := types.ListValueFrom(ctx, paramObjType, params)
	if diags.HasError() {
		t.Fatalf("build list: %v", diags)
	}

	var diagsVal diag.Diagnostics
	entries := buildParameterEntries(ctx, list, &diagsVal)
	if diagsVal.HasError() {
		t.Fatalf("build entries error: %v", diagsVal)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].Name != "innodb_buffer_pool_size" || entries[1].Value != "2G" {
		t.Errorf("entry[1]: %+v", entries[1])
	}
}

func TestUnit_ParamGroup_CreateAPI(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on("POST:/dbs/api/v1/configuration/parameter-group", func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "custom-pg" {
			t.Errorf("expected custom-pg, got %v", body["name"])
		}
		return "RAW", "Success", "Success"
	})

	r := &VDBSParameterGroupResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"name": "custom-pg",
	}
	resp, diags := callDBSAPI(context.Background(), r.client, http.MethodPost, pathParamGroupCreate, body)
	if diags.HasError() {
		t.Fatalf("callDBSAPI failed: %v", diags)
	}
	if resp.Message != "Success" {
		t.Errorf("expected success message, got %s", resp.Message)
	}
}

func TestUnit_ParamGroup_ResolveHostID(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on("POST:/dbs/api/v1/extend/vpc/customer/list", func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "RAW", "Success", []interface{}{
			map[string]interface{}{
				"id":        38983,
				"name":      "vpc-bced3f2e",
				"cmpHostId": 7,
			},
			map[string]interface{}{
				"id":        36960,
				"name":      "vpc-another",
				"cmpHostId": 6,
			},
		}
	})

	c := srv.newClient()
	hostID, err := resolveHostID(context.Background(), c, "cust-1", "38983")
	if err != nil {
		t.Fatalf("resolveHostID failed: %v", err)
	}
	if hostID != 7 {
		t.Errorf("expected hostID 7, got %d", hostID)
	}

	hostID2, err := resolveHostID(context.Background(), c, "cust-1", "36960")
	if err != nil {
		t.Fatalf("resolveHostID failed: %v", err)
	}
	if hostID2 != 6 {
		t.Errorf("expected hostID 6, got %d", hostID2)
	}
}

func TestUnit_ParamGroup_ResolveEngineVersion(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on("POST:/dbs/api/v1/extend/datastore/version/list", func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "mariadb" {
			t.Errorf("expected mariadb, got %v", body["name"])
		}
		return "RAW", "Success", []interface{}{
			map[string]interface{}{
				"id":   "3",
				"name": "10.5",
			},
			map[string]interface{}{
				"id":   "4",
				"name": "10.6",
			},
		}
	})

	c := srv.newClient()
	engine, versionID, err := resolveEngineVersion(context.Background(), c, "mariadb10.5", 7, "cust-1")
	if err != nil {
		t.Fatalf("resolveEngineVersion failed: %v", err)
	}
	if engine != "mariadb" || versionID != "3" {
		t.Errorf("expected mariadb/3, got %s/%s", engine, versionID)
	}
}

func TestUnit_ParamGroup_ReadAndMerge(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on("GET:/dbs/api/v1/configuration/parameter-group/pg-abc", func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "RAW", "Success", map[string]interface{}{
			"id":          "pg-abc",
			"name":        "custom-pg",
			"description": "Custom parameters",
			"datastore":   "mysql",
			"version":     "8.0",
			"vpcId":       100,
		}
	})

	srv.on("GET:/dbs/api/v1/configuration/parameter-group/pg-abc/parameters", func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "RAW", "Success", map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "max_connections", "value": "1000", "dataType": "numeric"},
			},
		}
	})

	r := &VDBSParameterGroupResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}

	paramObjType := types.ObjectType{AttrTypes: parameterAttrTypes}
	paramsList, _ := types.ListValueFrom(context.Background(), paramObjType, []ParameterModel{
		{Name: types.StringValue("max_connections"), Value: types.StringValue("")},
	})

	model := &VDBSParameterGroupResourceModel{
		ID:         types.StringValue("pg-abc"),
		VpcID:      types.StringValue("100"),
		Parameters: paramsList,
	}

	var diagsVal diag.Diagnostics
	found := r.readAndMerge(context.Background(), model, &diagsVal)
	if !found {
		t.Fatal("expected found=true")
	}
	if diagsVal.HasError() {
		t.Fatalf("unexpected diag: %v", diagsVal)
	}
	if model.Name.ValueString() != "custom-pg" {
		t.Errorf("name: %q", model.Name.ValueString())
	}
	if model.Family.ValueString() != "mysql8.0" {
		t.Errorf("family: %q", model.Family.ValueString())
	}

	var resParams []ParameterModel
	_ = model.Parameters.ElementsAs(context.Background(), &resParams, false)
	if len(resParams) != 1 || resParams[0].Value.ValueString() != "1000" {
		t.Errorf("expected max_connections=1000, got %+v", resParams)
	}
}

func TestUnit_ParamGroup_Import(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on("GET:/dbs/api/v1/configuration/parameter-group/pg-import", func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "RAW", "Success", map[string]interface{}{
			"id":          "pg-import",
			"name":        "imported-pg",
			"description": "Imported desc",
			"datastore":   "postgres",
			"version":     "14",
			"vpcId":       100,
		}
	})

	srv.on("GET:/dbs/api/v1/configuration/parameter-group/pg-import/parameters", func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "RAW", "Success", map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "work_mem", "value": "64MB", "dataType": "string"},
			},
		}
	})

	r := &VDBSParameterGroupResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}

	paramObjType := types.ObjectType{AttrTypes: parameterAttrTypes}
	paramsList, _ := types.ListValueFrom(context.Background(), paramObjType, []ParameterModel{
		{Name: types.StringValue("work_mem"), Value: types.StringValue("")},
	})

	model := &VDBSParameterGroupResourceModel{
		ID:         types.StringValue("pg-import"),
		VpcID:      types.StringValue("100"),
		Parameters: paramsList,
	}

	var diagsVal diag.Diagnostics
	found := r.readAndMerge(context.Background(), model, &diagsVal)
	if !found {
		t.Fatal("expected found=true for import")
	}

	var resParams []ParameterModel
	_ = model.Parameters.ElementsAs(context.Background(), &resParams, false)
	if len(resParams) != 1 || resParams[0].Value.ValueString() != "64MB" {
		t.Errorf("expected work_mem=64MB, got %+v", resParams)
	}
}

func TestUnit_ParamGroup_UpdateParameters(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on("GET:/dbs/api/v1/configuration/parameter-group/pg-123/parameters", func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return "RAW", "Success", map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "numeric_param", "dataType": "numeric"},
				map[string]interface{}{"name": "boolean_param", "dataType": "boolean"},
				map[string]interface{}{"name": "string_param", "dataType": "string"},
			},
		}
	})

	putCalled := false
	srv.on("PUT:/dbs/api/v1/configuration/parameter-group", func(body map[string]interface{}) (interface{}, string, interface{}) {
		putCalled = true
		params, ok := body["parameters"].(map[string]interface{})
		if !ok {
			t.Errorf("expected map for parameters, got %T", body["parameters"])
		}

		if params["numeric_param"] != float64(42) {
			t.Errorf("expected numeric_param to be float64(42), got %v (%T)", params["numeric_param"], params["numeric_param"])
		}
		if params["boolean_param"] != true {
			t.Errorf("expected boolean_param to be true, got %v (%T)", params["boolean_param"], params["boolean_param"])
		}
		if params["string_param"] != "hello" {
			t.Errorf("expected string_param to be 'hello', got %v (%T)", params["string_param"], params["string_param"])
		}
		return "RAW", "Success", "Success"
	})

	r := &VDBSParameterGroupResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}

	paramObjType := types.ObjectType{AttrTypes: parameterAttrTypes}
	paramsList, _ := types.ListValueFrom(context.Background(), paramObjType, []ParameterModel{
		{Name: types.StringValue("numeric_param"), Value: types.StringValue("42")},
		{Name: types.StringValue("boolean_param"), Value: types.StringValue("true")},
		{Name: types.StringValue("string_param"), Value: types.StringValue("hello")},
	})

	var diagsVal diag.Diagnostics
	r.updateParameters(context.Background(), "pg-123", 7, paramsList, &diagsVal)
	if diagsVal.HasError() {
		t.Fatalf("updateParameters failed: %v", diagsVal)
	}
	if !putCalled {
		t.Error("PUT /parameter-group handler was not called")
	}
}

// ---------- Data Source tests ----------

func TestUnit_DBInstanceDS_Read(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBInstanceDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", map[string]interface{}{
			"id":                 "dbs-001",
			"name":               "my-db",
			"status":             "ACTIVE",
			"flavorId":           "fl-1",
			"volumeSize":         float64(50),
			"dbSubnetGroupName":  "sg-group",
			"parameterGroupName": "pg-default",
		}
	})

	ds := &VDBSDatabaseInstanceDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"id":          "dbs-001",
		"vpc_id":      "100",
		"customer_id": "cust-1",
	}
	apiResp, d := callAPI(context.Background(), ds.client, pathDBInstanceDetail, body)
	if d.HasError() {
		t.Fatalf("unexpected error: %v", d)
	}
	// Verify response fields used by Read().
	import_json, _ := json.Marshal(apiResp.Data)
	var m map[string]interface{}
	_ = json.Unmarshal(import_json, &m)
	if asString(m, "name") != "my-db" {
		t.Errorf("name: got %q", asString(m, "name"))
	}
	if asString(m, "status") != "ACTIVE" {
		t.Errorf("status: got %q", asString(m, "status"))
	}
	if asInt64(m, "volumeSize") != 50 {
		t.Errorf("volumeSize: got %d", asInt64(m, "volumeSize"))
	}
}

func TestUnit_DBInstanceDS_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBInstanceDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(1), "INSTANCE_NOT_FOUND", nil
	})

	ds := &VDBSDatabaseInstanceDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"id":          "missing-id",
		"vpc_id":      "100",
		"customer_id": "cust-1",
	}
	apiResp, d := callAPI(context.Background(), ds.client, pathDBInstanceDetail, body)
	if !d.HasError() {
		t.Fatal("expected error for not-found, got none")
	}
	if apiResp == nil {
		t.Fatal("expected apiResp even on error")
	}
	if !isNotFoundMessage(apiResp.Message) {
		t.Errorf("expected not-found message, got %q", apiResp.Message)
	}
	_ = ds // suppress unused warning
}

func TestUnit_SubnetGroupDS_Read(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathSubnetGroupDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", map[string]interface{}{
			"id":        "sg-001",
			"name":      "db-subnet-group",
			"subnetIds": []interface{}{"sub-1", "sub-2"},
		}
	})

	ds := &VDBSSubnetGroupDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"id":          "sg-001",
		"vpc_id":      "100",
		"customer_id": "cust-1",
	}
	apiResp, d := callAPI(context.Background(), ds.client, pathSubnetGroupDetail, body)
	if d.HasError() {
		t.Fatalf("unexpected error: %v", d)
	}

	raw, _ := json.Marshal(apiResp.Data)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	if asString(m, "name") != "db-subnet-group" {
		t.Errorf("name: got %q", asString(m, "name"))
	}

	list, listDiags := listFromJSONArray(context.Background(), m, "subnetIds")
	if listDiags.HasError() {
		t.Fatalf("listFromJSONArray error: %v", listDiags)
	}
	if list.IsNull() || list.IsUnknown() {
		t.Fatal("subnet_ids should be populated")
	}
	var elems []types.String
	if diagErr := list.ElementsAs(context.Background(), &elems, false); diagErr.HasError() {
		t.Fatalf("ElementsAs: %v", diagErr)
	}
	if len(elems) != 2 {
		t.Errorf("expected 2 subnet IDs, got %d", len(elems))
	}
	_ = ds
}

func TestUnit_DBSGDS_Read(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)

	srv.on(pathDBSGDetail, func(_ map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "Success", map[string]interface{}{
			"id":          "dbsg-001",
			"name":        "db-sg",
			"description": "DB security group",
		}
	})

	ds := &VDBSSecurityGroupDataSource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: "100"}
	body := map[string]interface{}{
		"id":          "dbsg-001",
		"vpc_id":      "100",
		"customer_id": "cust-1",
	}
	apiResp, d := callAPI(context.Background(), ds.client, pathDBSGDetail, body)
	if d.HasError() {
		t.Fatalf("unexpected error: %v", d)
	}

	raw, _ := json.Marshal(apiResp.Data)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	if asString(m, "name") != "db-sg" {
		t.Errorf("name: got %q", asString(m, "name"))
	}
	if asString(m, "description") != "DB security group" {
		t.Errorf("description: got %q", asString(m, "description"))
	}
	_ = ds
}
