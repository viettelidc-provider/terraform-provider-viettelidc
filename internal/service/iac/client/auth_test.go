package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestExchangeATMForCMP verifies the two-step ATM->CMP exchange wires the
// right URLs, headers, body fields, and propagates the resulting tokens.
func TestExchangeATMForCMP(t *testing.T) {
	var sawAtmBody map[string]interface{}
	var sawOAuthBasicUser, sawOAuthBasicPass, sawOAuthGrant, sawOAuthCode, sawOAuthCT string
	var oauthQueryInternal string

	mux := http.NewServeMux()
	mux.HandleFunc("/iam/api/v1/authorization/via-atm-login", func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("via-atm-login Content-Type = %q, want application/json", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&sawAtmBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"key":"SUCCESS","data":"OLDJWT"}`))
	})
	mux.HandleFunc("/iam/oidc/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		oauthQueryInternal = r.URL.Query().Get("internal")
		sawOAuthCT = r.Header.Get("Content-Type")
		sawOAuthBasicUser, sawOAuthBasicPass, _ = r.BasicAuth()
		raw, _ := io.ReadAll(r.Body)
		// parse url-encoded body manually to avoid r.ParseForm caveats
		for _, kv := range strings.Split(string(raw), "&") {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				switch parts[0] {
				case "grant_type":
					sawOAuthGrant = parts[1]
				case "code":
					sawOAuthCode = parts[1]
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"NEWACCESS","refresh_token":"R","token_type":"Bearer","expires_in":1799}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	old, access, err := ExchangeATMForCMP(context.Background(), nil, srv.URL, ATMCredentials{
		Username:             "alice@viettelidc.com.vn",
		AtmToken:             "atm-blob",
		AtmAuthenticationTok: "uuid-1",
		VpcID:                "38545",
		CustomerID:           "244850",
	})
	if err != nil {
		t.Fatalf("ExchangeATMForCMP: %v", err)
	}
	if old != "OLDJWT" {
		t.Errorf("oldToken = %q, want OLDJWT", old)
	}
	if access != "NEWACCESS" {
		t.Errorf("accessToken = %q, want NEWACCESS", access)
	}

	// step 1 body
	if sawAtmBody["email"] != "alice@viettelidc.com.vn" {
		t.Errorf("email = %v", sawAtmBody["email"])
	}
	if sawAtmBody["atmToken"] != "atm-blob" {
		t.Errorf("atmToken = %v", sawAtmBody["atmToken"])
	}
	if sawAtmBody["atmRefreshToken"] != "uuid-1" {
		t.Errorf("atmRefreshToken = %v", sawAtmBody["atmRefreshToken"])
	}
	if sawAtmBody["planType"] != "start_cloud" {
		t.Errorf("planType = %v", sawAtmBody["planType"])
	}
	// JSON unmarshals numbers to float64
	if v, _ := sawAtmBody["vpcId"].(float64); v != 38545 {
		t.Errorf("vpcId = %v, want 38545 (number)", sawAtmBody["vpcId"])
	}
	if v, _ := sawAtmBody["customerId"].(float64); v != 244850 {
		t.Errorf("customerId = %v, want 244850 (number)", sawAtmBody["customerId"])
	}

	// step 2 mechanics
	if oauthQueryInternal != "true" {
		t.Errorf("oauth ?internal = %q, want true", oauthQueryInternal)
	}
	if sawOAuthCT != "application/x-www-form-urlencoded" {
		t.Errorf("oauth Content-Type = %q", sawOAuthCT)
	}
	if sawOAuthBasicUser != "cmp_apim" || sawOAuthBasicPass != "secret12345" {
		t.Errorf("oauth basic auth = %q:%q", sawOAuthBasicUser, sawOAuthBasicPass)
	}
	if sawOAuthGrant != "cmp_internal" {
		t.Errorf("oauth grant_type = %q, want cmp_internal", sawOAuthGrant)
	}
	if sawOAuthCode != "OLDJWT" {
		t.Errorf("oauth code = %q, want OLDJWT", sawOAuthCode)
	}
}

func TestExchangeATMForCMP_Step1FailureSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer srv.Close()
	_, _, err := ExchangeATMForCMP(context.Background(), nil, srv.URL, ATMCredentials{
		Username: "x", AtmToken: "a", AtmAuthenticationTok: "b", VpcID: "1", CustomerID: "2",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "via-atm-login") {
		t.Errorf("error should mention via-atm-login: %v", err)
	}
}
