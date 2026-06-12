package client

import (
	"encoding/json"
	"fmt"
)

// APIResponse is the uniform CSA envelope returned by API endpoints.
//
// Code may be encoded as a JSON number (decoded into float64) OR a JSON
// string (e.g. "SUCCESS", "FAILED"). IsSuccess() handles both via type
// switch — see Architecture Decision 5.
type APIResponse struct {
	Code    interface{}     `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ParseAPIResponse unmarshals the raw response body into a APIResponse.
//
// Three response shapes are handled:
//  1. Standard CSA envelope: {"code": 0, "message": "...", "data": {...}}
//     Used by fake-api and some real endpoints for error reporting.
//  2. Raw data (no envelope): {"pageIndex": -1, ..., "items": [...]}
//     Used by the real API backend for successful responses. When detected
//     (Code field is absent), Code is set to 0 and Data is set to the full
//     body so callers can parse it uniformly.
//  3. Error format: {"success": false, "errors": [{"key": "...", "params": [...]}]}
//     Used by the real API backend for validation errors. Code is set to -1
//     and Message is set to the first error key for consistent error reporting.
func ParseAPIResponse(body []byte) (*APIResponse, error) {
	var resp APIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		// Raw JSON array (e.g. listener-by-lb/all, pool-by-lb/all) — treat as success.
		if len(body) > 0 && body[0] == '[' {
			return &APIResponse{Code: float64(0), Data: json.RawMessage(body)}, nil
		}
		return nil, fmt.Errorf("api client: parse response: %w", err)
	}
	if resp.Code == nil {
		// Detect {"success": false, "errors": [...]} error format from real API.
		// Use *bool so we can distinguish "success field absent" from "success=false".
		// When success is absent the response is a raw data payload, not an error.
		var errBody struct {
			Success *bool `json:"success"`
			Errors  []struct {
				Key string `json:"key"`
			} `json:"errors"`
		}
		if err2 := json.Unmarshal(body, &errBody); err2 == nil && errBody.Success != nil && !*errBody.Success {
			resp.Code = float64(-1)
			if len(errBody.Errors) > 0 {
				resp.Message = errBody.Errors[0].Key
			} else {
				resp.Message = "API reported failure (no error details)"
			}
			return &resp, nil
		}
		// Raw success response — treat entire body as data payload.
		resp.Code = float64(0)
		resp.Data = json.RawMessage(body)
	}
	return &resp, nil
}

// IsSuccess reports whether the CSA envelope indicates a successful operation.
//
// Type-switch is REQUIRED because JSON decode produces:
//   - float64 for numeric "code" (Go's default for json.Unmarshal into interface{})
//   - string  for textual "code"
//
// DO NOT rewrite as "code != 0 || code != \"SUCCESS\"" — that condition is
// always true (logic bug from earlier prototype).
func (r *APIResponse) IsSuccess() bool {
	if r == nil {
		return false
	}
	switch v := r.Code.(type) {
	case float64:
		return v == 0
	case string:
		return v == "SUCCESS"
	default:
		return false
	}
}

// IsAPISuccess is a free-function alias kept for API parity with the spec
// in Story 1.2 AC#4.
func IsAPISuccess(r *APIResponse) bool { return r.IsSuccess() }

// ExtractData unmarshals the Data field into the provided destination
// pointer. Returns an error if Data is empty/null or unmarshalling fails.
func (r *APIResponse) ExtractData(dst interface{}) error {
	if r == nil || len(r.Data) == 0 || string(r.Data) == "null" {
		return fmt.Errorf("api client: response data is empty")
	}
	if err := json.Unmarshal(r.Data, dst); err != nil {
		return fmt.Errorf("api client: extract data: %w", err)
	}
	return nil
}
