// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Endpoints (relative to API base URL).
const (
	pathLogin      = "/iam/api/v1/authorization/login"
	pathOAuthToken = "/iam/oidc/oauth2/token?internal=true"

	// Basic-auth credentials baked into the CMP web client for the
	// internal OAuth2 token endpoint. Sourced from the CMP frontend bundle
	// (same values shipped to the browser).
	cmpAPIMUser = "cmp_apim"
	cmpAPIMPass = "secret12345"
)

// LoginCredentials holds email/password for the direct login path.
type LoginCredentials struct {
	Username string // email (root user) or IAM username
	Password string
	UserType string // "ROOT_USER" (default) or "IAM_USER"
	DomainId string // required for IAM_USER, ignored for ROOT_USER
}

func oauthExchange(ctx context.Context, httpClient *http.Client, baseURL, oldToken string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "cmp_internal")
	form.Set("code", oldToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+pathOAuthToken, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(cmpAPIMUser, cmpAPIMPass)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read oauth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 512))
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("empty access_token in response")
	}
	return tok.AccessToken, nil
}

// ExchangeATMForCMP performs the two-step email/password login flow:
//
//  1. POST /iam/api/v1/authorization/login → returns a JWT in {"data":"..."};
//     this becomes Authorization: Bearer ... for downstream CSA calls.
//  2. POST /iam/oidc/oauth2/token?internal=true with grant_type=cmp_internal,
//     basic-authed as cmp_apim → access_token becomes X-Authorization: Bearer ...
func LoginWithPassword(ctx context.Context, httpClient *http.Client, baseURL string, c LoginCredentials) (oldToken, accessToken string, err error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	baseURL = strings.TrimRight(baseURL, "/")
	userType := c.UserType
	if userType == "" {
		userType = "ROOT_USER"
	}

	body := map[string]interface{}{
		"password":  c.Password,
		"user_type": userType,
	}
	if userType == "IAM_USER" {
		body["username"] = c.Username
	} else {
		body["email"] = c.Username
	}
	if c.DomainId != "" {
		body["domain_id"] = c.DomainId
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+pathLogin, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("login: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("login: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("login: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 512))
	}
	var env struct {
		Code int    `json:"code"`
		Key  string `json:"key"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(respBody, &env); err != nil {
		return "", "", fmt.Errorf("login: decode response: %w (body=%s)", err, truncate(string(respBody), 256))
	}
	if env.Data == "" {
		return "", "", fmt.Errorf("login: empty data field in response (code=%d key=%q)", env.Code, env.Key)
	}

	oldToken = env.Data
	accessToken, err = oauthExchange(ctx, httpClient, baseURL, oldToken)
	if err != nil {
		return "", "", fmt.Errorf("login oauth exchange: %w", err)
	}
	return oldToken, accessToken, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ExtractCustomerIDFromJWT decodes the JWT payload (second segment) and returns
// the customer_id claim. The JWT is not verified — this is used only to avoid
// requiring the user to redundantly specify customer_id when it is already
// embedded in the token they obtained during login.
//
// Returns "" and no error when the claim is absent so the caller can decide
// whether to treat that as an error.
func ExtractCustomerIDFromJWT(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}
	// JWT uses base64url encoding without padding.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Fallback for non-standard JWTs with padding
		padStr := parts[1]
		if len(padStr)%4 != 0 {
			padStr += strings.Repeat("=", 4-len(padStr)%4)
		}
		payload, err = base64.URLEncoding.DecodeString(padStr)
		if err != nil {
			// Final fallback: some broken tokens use standard base64 instead of URL safe
			payload, err = base64.StdEncoding.DecodeString(padStr)
			if err != nil {
				return "", fmt.Errorf("decode JWT payload: %w", err)
			}
		}
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse JWT claims: %w", err)
	}
	// The field may be "customer_id" or "customerId" depending on the IAM version.
	for _, key := range []string{"customer_id", "customerId"} {
		if v, ok := claims[key]; ok {
			switch id := v.(type) {
			case float64:
				return strconv.Itoa(int(id)), nil
			case string:
				if id != "" {
					return id, nil
				}
			}
		}
	}
	return "", nil
}

// ATMCredentials holds the ATM-token-based credentials for the via-atm-login flow.
type ATMCredentials struct {
	Username             string // email
	AtmToken             string // ATM blob token
	AtmAuthenticationTok string // ATM authentication token / UUID
	VpcID                string // numeric VPC ID as string
	CustomerID           string // numeric customer ID as string
}

const pathViaAtmLogin = "/iam/api/v1/authorization/via-atm-login"

// ExchangeATMForCMP performs the two-step ATM-token login flow:
//
//  1. POST /iam/api/v1/authorization/via-atm-login → returns a JWT in {"data":"..."};
//     this becomes Authorization: Bearer ... for downstream CSA calls.
//  2. POST /iam/oidc/oauth2/token?internal=true with grant_type=cmp_internal,
//     basic-authed as cmp_apim → access_token becomes X-Authorization: Bearer ...
func ExchangeATMForCMP(ctx context.Context, httpClient *http.Client, baseURL string, c ATMCredentials) (oldToken, accessToken string, err error) {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	baseURL = strings.TrimRight(baseURL, "/")

	vpcID, err := strconv.Atoi(c.VpcID)
	if err != nil {
		return "", "", fmt.Errorf("via-atm-login: invalid VpcID %q: %w", c.VpcID, err)
	}
	customerID, err := strconv.Atoi(c.CustomerID)
	if err != nil {
		return "", "", fmt.Errorf("via-atm-login: invalid CustomerID %q: %w", c.CustomerID, err)
	}

	body := map[string]interface{}{
		"email":           c.Username,
		"atmToken":        c.AtmToken,
		"atmRefreshToken": c.AtmAuthenticationTok,
		"planType":        "start_cloud",
		"vpcId":           vpcID,
		"customerId":      customerID,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+pathViaAtmLogin, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("via-atm-login: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("via-atm-login: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("via-atm-login: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 512))
	}
	var env struct {
		Code int    `json:"code"`
		Key  string `json:"key"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(respBody, &env); err != nil {
		return "", "", fmt.Errorf("via-atm-login: decode response: %w (body=%s)", err, truncate(string(respBody), 256))
	}
	if env.Data == "" {
		return "", "", fmt.Errorf("via-atm-login: empty data field in response (code=%d key=%q)", env.Code, env.Key)
	}

	oldToken = env.Data
	accessToken, err = oauthExchange(ctx, httpClient, baseURL, oldToken)
	if err != nil {
		return "", "", fmt.Errorf("via-atm-login oauth exchange: %w", err)
	}
	return oldToken, accessToken, nil
}
