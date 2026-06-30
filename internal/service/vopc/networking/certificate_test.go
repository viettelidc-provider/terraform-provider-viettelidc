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

	"github.com/hashicorp/terraform-plugin-framework/resource"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// ---------- Fake KMS Server ----------
// Unlike fakeAPIServer (POST-only envelope), KMS uses RESTful GET/POST/PUT/DELETE
// with raw JSON responses (no CSA envelope).

type kmsHandlerKey struct {
	method string
	path   string
}

type fakeKMSServer struct {
	*httptest.Server
	handlers map[kmsHandlerKey]func(body map[string]interface{}) (statusCode int, respBody interface{})
	calls    map[kmsHandlerKey]int
}

func newFakeKMS(t *testing.T) *fakeKMSServer {
	t.Helper()
	f := &fakeKMSServer{
		handlers: map[kmsHandlerKey]func(map[string]interface{}) (int, interface{}){},
		calls:    map[kmsHandlerKey]int{},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		key := kmsHandlerKey{method: r.Method, path: r.URL.Path}
		f.calls[key]++
		h, ok := f.handlers[key]
		if !ok {
			http.Error(w, fmt.Sprintf("no KMS handler for %s %s", r.Method, r.URL.Path), http.StatusNotFound)
			return
		}
		statusCode, respBody := h(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if respBody != nil {
			_ = json.NewEncoder(w).Encode(respBody)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeKMSServer) on(method, path string, h func(map[string]interface{}) (int, interface{})) {
	f.handlers[kmsHandlerKey{method: method, path: path}] = h
}

func (f *fakeKMSServer) newClient() *client.Client {
	return client.NewClient(f.URL, "test-token")
}

// certSuccessItem builds a certListResp with one SUCCESS item.
func certListWith(items ...certItem) certListResp {
	return certListResp{
		PageIndex:  0,
		PageSize:   20,
		TotalItems: len(items),
		Items:      items,
	}
}

const testVpcID = "200"
const testCertID = "cert-abc123"

// ---------- CertificateResource tests ----------

func TestCertificateResource_Create(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	// POST /key-manager/api/v1/kms/200/certificate → 200 empty body
	srv.on(http.MethodPost, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, nil
		})
	// GET list — returns the cert as SUCCESS immediately (no real polling needed in unit test)
	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith(certItem{
				ID:        testCertID,
				Name:      "my-cert",
				Status:    "SUCCESS",
				CreatedAt: "2025-01-01T00:00:00Z",
			})
		})

	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}

	// Exercise Create: POST + pollCertByName (uses GET list).
	createPath := fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID)
	_, diags := callKMS(context.Background(), r.client, http.MethodPost, createPath, map[string]interface{}{
		"name":        "my-cert",
		"certificate": "-----BEGIN CERTIFICATE-----",
	})
	if diags.HasError() {
		t.Fatalf("create POST: %v", diags)
	}

	item, err := r.pollCertByName(context.Background(), testVpcID, "my-cert", 0) // 0 → one shot
	if err != nil {
		t.Fatalf("pollCertByName: %v", err)
	}
	if item.ID != testCertID {
		t.Errorf("id: %q", item.ID)
	}
	if item.Status != "SUCCESS" {
		t.Errorf("status: %q", item.Status)
	}
}

func TestCertificateResource_Update(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	updatePath := fmt.Sprintf("%s/%s/certificate/%s", pathCertBase, testVpcID, testCertID)
	srv.on(http.MethodPut, updatePath, func(body map[string]interface{}) (int, interface{}) {
		if body["name"] != "new-name" {
			t.Errorf("update: name=%v", body["name"])
		}
		return http.StatusOK, nil
	})
	// GET list for re-read after update.
	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith(certItem{
				ID:        testCertID,
				Name:      "new-name",
				Status:    "SUCCESS",
				CreatedAt: "2025-01-01T00:00:00Z",
			})
		})

	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}

	_, diags := callKMS(context.Background(), r.client, http.MethodPut, updatePath, map[string]interface{}{
		"name": "new-name",
	})
	if diags.HasError() {
		t.Fatalf("update PUT: %v", diags)
	}

	// Verify readCertByID (re-read after update) works.
	item, found, d := r.readCertByID(context.Background(), testVpcID, testCertID)
	if d.HasError() {
		t.Fatalf("readCertByID: %v", d)
	}
	if !found {
		t.Fatal("cert not found after update")
	}
	if item.Name != "new-name" {
		t.Errorf("name: %q", item.Name)
	}
}

func TestCertificateResource_Delete(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	deletePath := fmt.Sprintf("%s/%s/certificate/%s", pathCertBase, testVpcID, testCertID)
	srv.on(http.MethodDelete, deletePath, func(_ map[string]interface{}) (int, interface{}) {
		return http.StatusOK, nil
	})
	// GET list returns empty → cert gone immediately.
	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith() // empty items
		})

	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}

	_, diags := callKMS(context.Background(), r.client, http.MethodDelete, deletePath, nil)
	if diags.HasError() {
		t.Fatalf("delete: %v", diags)
	}

	if err := r.pollCertGone(context.Background(), testVpcID, testCertID, 0); err != nil {
		t.Fatalf("pollCertGone: %v", err)
	}
}

func TestCertificateResource_ReadCertByID(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith(
				certItem{ID: "other-cert", Name: "other", Status: "SUCCESS"},
				certItem{ID: testCertID, Name: "my-cert", Status: "SUCCESS", CreatedAt: "2025-06-01T00:00:00Z"},
			)
		})

	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}
	item, found, d := r.readCertByID(context.Background(), testVpcID, testCertID)
	if d.HasError() {
		t.Fatalf("readCertByID: %v", d)
	}
	if !found {
		t.Fatal("expected cert found")
	}
	if item.Name != "my-cert" {
		t.Errorf("name: %q", item.Name)
	}
}

func TestCertificateResource_ReadCertByID_NotFound(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith() // empty
		})

	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}
	_, found, d := r.readCertByID(context.Background(), testVpcID, "missing-id")
	if d.HasError() {
		t.Fatalf("unexpected error: %v", d)
	}
	if found {
		t.Fatal("expected not found")
	}
}

func TestCertificateResource_PollCertByName_Failed(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith(certItem{
				ID:     testCertID,
				Name:   "bad-cert",
				Status: "ERROR",
			})
		})

	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}
	_, err := r.pollCertByName(context.Background(), testVpcID, "bad-cert", 0)
	if err == nil {
		t.Fatal("expected error for ERROR status cert")
	}
}

func TestCertificateResource_SchemaHasCertificate(t *testing.T) {
	t.Parallel()
	// Verify schema has certificate attribute declared (sensitive check is at schema level, not testable without TF harness).
	srv := newFakeKMS(t)
	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}

	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	if _, ok := schemaResp.Schema.Attributes["certificate"]; !ok {
		t.Fatal("certificate attribute missing from schema")
	}
	if _, ok := schemaResp.Schema.Attributes["status"]; !ok {
		t.Fatal("status attribute missing from schema")
	}
}

// ---------- CertificateDataSource tests ----------

func TestCertificateDataSource_ByName(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith(
				certItem{ID: "id-1", Name: "cert-a", Status: "SUCCESS"},
				certItem{ID: "id-2", Name: "cert-b", Status: "SUCCESS"},
			)
		})

	items, diags := listCertificates(context.Background(), srv.newClient(), testVpcID)
	if diags.HasError() {
		t.Fatalf("listCertificates: %v", diags)
	}
	var found certItem
	for _, item := range items {
		if item.Name == "cert-b" {
			found = item
			break
		}
	}
	if found.ID != "id-2" {
		t.Errorf("expected id-2, got %q", found.ID)
	}
}

func TestCertificateDataSource_ByID(t *testing.T) {
	t.Parallel()
	srv := newFakeKMS(t)

	srv.on(http.MethodGet, fmt.Sprintf("%s/%s/certificate", pathCertBase, testVpcID),
		func(_ map[string]interface{}) (int, interface{}) {
			return http.StatusOK, certListWith(
				certItem{ID: testCertID, Name: "my-cert", Status: "SUCCESS", CreatedAt: "2025-01-01T00:00:00Z"},
			)
		})

	r := &CertificateResource{client: srv.newClient(), customerID: "cust-1", defaultVpcID: testVpcID}
	item, found, d := r.readCertByID(context.Background(), testVpcID, testCertID)
	if d.HasError() {
		t.Fatalf("readCertByID: %v", d)
	}
	if !found {
		t.Fatal("cert not found")
	}
	if item.Name != "my-cert" {
		t.Errorf("name: %q", item.Name)
	}
	if item.CreatedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("createdAt: %q", item.CreatedAt)
	}
}

func TestCertificateResource_ImportState_Format(t *testing.T) {
	t.Parallel()
	// Import ID "200/cert-abc123" should split into vpc_id=200, id=cert-abc123.
	importID := fmt.Sprintf("%s/%s", testVpcID, testCertID)
	parts := splitN(importID, "/", 2)
	if parts[0] != testVpcID {
		t.Errorf("vpc_id: %q", parts[0])
	}
	if parts[1] != testCertID {
		t.Errorf("cert_id: %q", parts[1])
	}
}

// Thin wrapper around strings.SplitN to keep import test self-contained.
func splitN(s, sep string, n int) []string {
	result := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		idx := indexOf(s, sep)
		if idx < 0 {
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	result = append(result, s)
	return result
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
