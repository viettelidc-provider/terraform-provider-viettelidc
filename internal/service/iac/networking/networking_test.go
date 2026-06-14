package networking

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

// fakeAPIServer routes path → handler returning the (code,message,data) envelope.
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
		{"plan wins", "plan-vpc", "default-vpc", "plan-vpc", false},
		{"falls back to default", "", "default-vpc", "default-vpc", false},
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
	yes := []string{"resource not found", "Subnet does not exist", "no such NIC", "does not exist on server"}
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

func TestIsNotAttachedMessage(t *testing.T) {
	t.Parallel()
	if !isNotAttachedMessage("nic is not attached") {
		t.Error("want true")
	}
	if isNotAttachedMessage("internal error") {
		t.Error("want false")
	}
}

func TestBuildSubnetCreateBody(t *testing.T) {
	t.Parallel()
	plan := SubnetResourceModel{
		Name:           types.StringValue("net-a"),
		NetworkAddress: types.StringValue("10.0.0.0/24"),
		IsPublicZone:   types.BoolValue(true),
		Description:    types.StringValue("hi"),
	}
	body := buildSubnetCreateBody(plan, "cust", "vpc1")
	if body["name"] != "net-a" || body["vpc_id"] != "vpc1" || body["customer_id"] != "cust" {
		t.Fatalf("unexpected body: %#v", body)
	}
	if body["is_public_zone"] != true {
		t.Fatalf("is_public_zone=%v", body["is_public_zone"])
	}
	if body["description"] != "hi" {
		t.Fatalf("description missing: %#v", body)
	}

	plan.Description = types.StringValue("")
	body = buildSubnetCreateBody(plan, "cust", "vpc1")
	if _, ok := body["description"]; ok {
		t.Fatal("empty description should be omitted")
	}
}

func TestBuildNicCreateBody_DynamicOmitsIP(t *testing.T) {
	t.Parallel()
	plan := NetworkInterfaceResourceModel{
		Name:         types.StringValue("nic-1"),
		SubnetID:     types.StringValue("sub-1"),
		IpAssignType: types.StringValue(ipAssignDynamic),
		IpAddress:    types.StringValue("1.2.3.4"), // user shouldn't supply but defensive
	}
	body := buildNicCreateBody(plan, "cust", "vpc1")
	if _, ok := body["ip_address"]; ok {
		t.Fatal("DYNAMIC must not send ip_address")
	}
}

func TestBuildNicCreateBody_StaticIncludesIP(t *testing.T) {
	t.Parallel()
	plan := NetworkInterfaceResourceModel{
		Name:         types.StringValue("nic-1"),
		SubnetID:     types.StringValue("sub-1"),
		IpAssignType: types.StringValue(ipAssignStatic),
		IpAddress:    types.StringValue("10.0.0.5"),
	}
	body := buildNicCreateBody(plan, "cust", "vpc1")
	if body["ip_address"] != "10.0.0.5" {
		t.Fatalf("expected ip_address 10.0.0.5, got %v", body["ip_address"])
	}
}

func TestApplyNicFilters(t *testing.T) {
	t.Parallel()
	items := []map[string]interface{}{
		{"name": "a", "vttSubnetId": "s1", "status": "ACTIVE"},
		{"name": "b", "vttSubnetId": "s2", "status": "ACTIVE"},
		{"name": "a", "vttSubnetId": "s2", "status": "STOPPED"},
	}
	got := applyNicFilters(items, &NicFilters{Name: types.StringValue("a")})
	if len(got) != 2 {
		t.Fatalf("want 2 by name, got %d", len(got))
	}
	got = applyNicFilters(items, &NicFilters{SubnetID: types.StringValue("s2"), Status: types.StringValue("ACTIVE")})
	if len(got) != 1 || got[0]["name"] != "b" {
		t.Fatalf("subnet+status filter wrong: %#v", got)
	}
	got = applyNicFilters(items, nil)
	if len(got) != 3 {
		t.Fatalf("nil filter should pass through")
	}
}

func TestParseAndBuildAttachmentID(t *testing.T) {
	t.Parallel()
	id := buildAttachmentID("nic-1", "vm-1")
	if id != "nic-1/vm-1" {
		t.Fatalf("want nic-1/vm-1, got %s", id)
	}
	nic, vm, err := parseAttachmentID(id)
	if err != nil || nic != "nic-1" || vm != "vm-1" {
		t.Fatalf("parse failed: %v %v %v", nic, vm, err)
	}
	if _, _, err := parseAttachmentID("bad"); err == nil {
		t.Error("expected parse error for 'bad'")
	}
	if _, _, err := parseAttachmentID("/vm"); err == nil {
		t.Error("expected parse error for empty nic")
	}
	if _, _, err := parseAttachmentID("nic/"); err == nil {
		t.Error("expected parse error for empty vm")
	}
	// Allow slashes in instance id (SplitN N=2).
	nic, vm, err = parseAttachmentID("nic-1/vm/with/slash")
	if err != nil || nic != "nic-1" || vm != "vm/with/slash" {
		t.Fatalf("composite parse failed: %v %v %v", nic, vm, err)
	}
}

func TestExtractSubnetID(t *testing.T) {
	t.Parallel()
	resp := &client.APIResponse{Data: json.RawMessage(`{"vttSubnetId":"sub-42"}`)}
	id, err := extractSubnetID(resp)
	if err != nil || id != "sub-42" {
		t.Fatalf("got %q err=%v", id, err)
	}
	resp = &client.APIResponse{Data: json.RawMessage(`{"name":"foo"}`)}
	if _, err := extractSubnetID(resp); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestExtractNicID(t *testing.T) {
	t.Parallel()
	resp := &client.APIResponse{Data: json.RawMessage(`{"vttNetworkInterfaceId":"nic-9"}`)}
	id, err := extractNicID(resp)
	if err != nil || id != "nic-9" {
		t.Fatalf("got %q err=%v", id, err)
	}
}

func TestMapSubnetResponse(t *testing.T) {
	t.Parallel()
	resp := &client.APIResponse{Data: json.RawMessage(`{
		"vttSubnetId":"s1","name":"net","networkAddress":"10.0.0.0/24",
		"isPublicZone":true,"vpcId":"v1","description":"desc"
	}`)}
	m := &SubnetResourceModel{}
	if err := mapSubnetResponse(resp, m); err != nil {
		t.Fatal(err)
	}
	if m.ID.ValueString() != "s1" || m.Name.ValueString() != "net" || !m.IsPublicZone.ValueBool() {
		t.Fatalf("model wrong: %#v", m)
	}
	if m.Description.ValueString() != "desc" {
		t.Fatalf("desc: %q", m.Description.ValueString())
	}
}

func TestDecodeSubnetList_ArrayAndObject(t *testing.T) {
	t.Parallel()
	r := &client.APIResponse{Data: json.RawMessage(`[{"name":"a"},{"name":"b"}]`)}
	got, err := decodeSubnetList(r)
	if err != nil || len(got) != 2 {
		t.Fatalf("array shape: %v err=%v", got, err)
	}
	r = &client.APIResponse{Data: json.RawMessage(`{"subnets":[{"name":"x"}]}`)}
	got, err = decodeSubnetList(r)
	if err != nil || len(got) != 1 || got[0]["name"] != "x" {
		t.Fatalf("object shape: %v err=%v", got, err)
	}
}

func TestReadAttachedInstance(t *testing.T) {
	t.Parallel()
	r := &client.APIResponse{Data: json.RawMessage(`{"attachedInstanceId":"vm-1"}`)}
	attached, id, err := readAttachedInstance(r)
	if err != nil || !attached || id != "vm-1" {
		t.Fatalf("got attached=%v id=%q err=%v", attached, id, err)
	}
	r = &client.APIResponse{Data: json.RawMessage(`{}`)}
	attached, _, _ = readAttachedInstance(r)
	if attached {
		t.Fatal("expected not attached for empty payload")
	}
}

// ---------- httptest-backed integration tests ----------

func TestSubnetResource_CreateRead(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathSubnetCreate, func(body map[string]interface{}) (interface{}, string, interface{}) {
		if body["name"] != "net-a" {
			t.Errorf("create body name wrong: %v", body["name"])
		}
		if body["vpc_id"] != "vpc-1" || body["customer_id"] != "cust" {
			t.Errorf("create body context wrong: %#v", body)
		}
		return float64(0), "ok", map[string]interface{}{"vttSubnetId": "sub-1"}
	})
	srv.on(pathSubnetDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return float64(0), "ok", map[string]interface{}{
			"vttSubnetId":    "sub-1",
			"name":           "net-a",
			"networkAddress": "10.0.0.0/24",
			"isPublicZone":   false,
			"vpcId":          "vpc-1",
			"description":    "",
		}
	})
	r := &SubnetResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}

	// Drive the helper-level create-then-read flow (Framework integration
	// requires schema marshal which is exercised by acceptance tests).
	plan := SubnetResourceModel{
		Name:           types.StringValue("net-a"),
		NetworkAddress: types.StringValue("10.0.0.0/24"),
		IsPublicZone:   types.BoolValue(false),
		VpcID:          types.StringValue(""),
		Description:    types.StringValue(""),
	}
	body := buildSubnetCreateBody(plan, r.customerID, r.defaultVpcID)
	resp, diags := callAPI(context.Background(), r.client, pathSubnetCreate, body)
	if diags.HasError() {
		t.Fatalf("create call failed: %v", diags)
	}
	id, err := extractSubnetID(resp)
	if err != nil || id != "sub-1" {
		t.Fatalf("extract: %v %v", id, err)
	}
	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(r.defaultVpcID)

	var dgs diag.Diagnostics
	if !r.readInto(context.Background(), &plan, &dgs) {
		t.Fatal("readInto reported drift unexpectedly")
	}
	if dgs.HasError() {
		t.Fatalf("readInto diag: %v", dgs)
	}
	if plan.Name.ValueString() != "net-a" || plan.NetworkAddress.ValueString() != "10.0.0.0/24" {
		t.Fatalf("model after read: %#v", plan)
	}
	if srv.calls[pathSubnetCreate] != 1 || srv.calls[pathSubnetDetail] != 1 {
		t.Fatalf("call counts wrong: %#v", srv.calls)
	}
}

func TestSubnetResource_ReadDriftReturnsFalse(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathSubnetDetail, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return "FAILURE", "Subnet not found", nil
	})
	r := &SubnetResource{client: srv.newClient(), customerID: "c", defaultVpcID: "v"}
	m := &SubnetResourceModel{ID: types.StringValue("x"), VpcID: types.StringValue("v")}
	var dgs diag.Diagnostics
	if r.readInto(context.Background(), m, &dgs) {
		t.Fatal("expected drift (false) on not-found")
	}
	if dgs.HasError() {
		t.Fatalf("drift should not produce error diag: %v", dgs)
	}
}

func TestSubnetResource_DeleteIdempotent(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathSubnetDelete, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return "FAILURE", "Subnet not found", nil
	})
	r := &SubnetResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	body := map[string]interface{}{"subnet_id": "sub-x", "vpc_id": "vpc-1", "customer_id": "cust"}
	resp, diags := callAPI(context.Background(), r.client, pathSubnetDelete, body)
	if !diags.HasError() {
		t.Fatal("expected error diag from CSA failure envelope")
	}
	if resp == nil || !isNotFoundMessage(resp.Message) {
		t.Fatalf("expected not-found message, got %#v", resp)
	}
}

func TestNicAttachment_DeleteIdempotent(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathNicDetach, func(body map[string]interface{}) (interface{}, string, interface{}) {
		return "FAILURE", "NIC is not attached to instance", nil
	})
	r := &NetworkInterfaceAttachmentResource{client: srv.newClient(), customerID: "cust", defaultVpcID: "vpc-1"}
	body := map[string]interface{}{
		"network_interface_id": "nic-1",
		"instance_id":          "vm-1",
		"vpc_id":               "vpc-1",
		"customer_id":          r.customerID,
	}
	resp, diags := callAPI(context.Background(), r.client, pathNicDetach, body)
	if !diags.HasError() {
		t.Fatal("expected diag error")
	}
	if !isNotAttachedMessage(resp.Message) {
		t.Fatalf("expected not-attached, got %q", resp.Message)
	}
	_ = r
}

func TestSubnetsList_DecodeAndCount(t *testing.T) {
	t.Parallel()
	srv := newFakeAPI(t)
	srv.on(pathSubnetList, func(body map[string]interface{}) (interface{}, string, interface{}) {
		out := make([]map[string]interface{}, 3)
		for i := range out {
			out[i] = map[string]interface{}{
				"vttSubnetId":    "s" + string(rune('A'+i)),
				"name":           "n" + string(rune('A'+i)),
				"networkAddress": "10.0.0.0/24",
				"isPublicZone":   false,
				"description":    "",
			}
		}
		return float64(0), "ok", out
	})
	c := srv.newClient()
	resp, diags := callAPI(context.Background(), c, pathSubnetList, map[string]interface{}{"vpc_id": "v1", "customer_id": "c1"})
	if diags.HasError() {
		t.Fatalf("unexpected: %v", diags)
	}
	items, err := decodeSubnetList(resp)
	if err != nil || len(items) != 3 {
		t.Fatalf("decode: %v err=%v", items, err)
	}
}
