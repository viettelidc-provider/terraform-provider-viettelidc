// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import "testing"

func TestParseAPIResponse_Valid(t *testing.T) {
	body := []byte(`{"code":0,"message":"OK","data":{"id":"abc"}}`)
	r, err := ParseAPIResponse(body)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !r.IsSuccess() {
		t.Errorf("expected success for code:0")
	}
	if r.Message != "OK" {
		t.Errorf("Message = %q, want OK", r.Message)
	}
}

func TestParseAPIResponse_Invalid(t *testing.T) {
	if _, err := ParseAPIResponse([]byte(`not json`)); err == nil {
		t.Error("expected parse error")
	}
}

func TestIsSuccess_Float64Zero(t *testing.T) {
	r := &APIResponse{Code: float64(0)}
	if !r.IsSuccess() {
		t.Error("float64(0) should be success")
	}
}

func TestIsSuccess_Float64NonZero(t *testing.T) {
	r := &APIResponse{Code: float64(1)}
	if r.IsSuccess() {
		t.Error("float64(1) should NOT be success")
	}
}

func TestIsSuccess_StringSUCCESS(t *testing.T) {
	r := &APIResponse{Code: "SUCCESS"}
	if !r.IsSuccess() {
		t.Error(`"SUCCESS" should be success`)
	}
}

func TestIsSuccess_StringFailed(t *testing.T) {
	r := &APIResponse{Code: "FAILED"}
	if r.IsSuccess() {
		t.Error(`"FAILED" should NOT be success`)
	}
}

func TestIsSuccess_NilCode(t *testing.T) {
	r := &APIResponse{Code: nil}
	if r.IsSuccess() {
		t.Error("nil code should NOT be success")
	}
}

func TestIsSuccess_NilReceiver(t *testing.T) {
	var r *APIResponse
	if r.IsSuccess() {
		t.Error("nil receiver should NOT be success")
	}
}

// JSON decode of integer code MUST land in float64 branch.
func TestIsSuccess_JSONIntegerCode(t *testing.T) {
	r, err := ParseAPIResponse([]byte(`{"code":0,"message":"","data":null}`))
	if err != nil {
		t.Fatal(err)
	}
	// Confirm Go decoded the JSON number as float64 (not int).
	if _, ok := r.Code.(float64); !ok {
		t.Fatalf("code type = %T, want float64", r.Code)
	}
	if !r.IsSuccess() {
		t.Error("code:0 should be success")
	}
}

func TestIsAPISuccess_Alias(t *testing.T) {
	r := &APIResponse{Code: float64(0)}
	if !IsAPISuccess(r) {
		t.Error("free function should match method")
	}
}

func TestExtractData_OK(t *testing.T) {
	r, err := ParseAPIResponse([]byte(`{"code":0,"data":{"id":"abc","n":42}}`))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		ID string `json:"id"`
		N  int    `json:"n"`
	}
	if err := r.ExtractData(&out); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if out.ID != "abc" || out.N != 42 {
		t.Errorf("got %+v", out)
	}
}

func TestExtractData_Empty(t *testing.T) {
	r, _ := ParseAPIResponse([]byte(`{"code":0,"data":null}`))
	var out struct{}
	if err := r.ExtractData(&out); err == nil {
		t.Error("expected error on null data")
	}
}
