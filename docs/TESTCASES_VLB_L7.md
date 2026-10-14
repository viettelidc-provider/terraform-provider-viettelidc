# vLB Layer 7 Features — Test Case Suite

> **Live Cloud Credentials**
> - Account: `email = "tandt@viettelidc.com.vn"`, `password = "Comopo2026@"`, `host_id = 7`, `customerId = 238250`
> - VPC: `40784`
> - Load Balancer: `1059`
> - Listener: `2567` (protocol: HTTP or TERMINATED_HTTPS, status: success)
> - Pool: `72799` (available for L7)
> - Existing Policy on listener 2567: ID `6`, name `"dev"`, action `"REDIRECT_TO_URL"`, redirectUrl `"https://dev.com"`, status `"success"`.

---

## Group 1: L7 Policy CRUD

### TC-POL-001: Create Policy with Action REJECT
- **Input**: `listener_id = 2567`, `name = "reject-test"`, `action = "REJECT"`
- **Expected**: Policy created with `status = "pending"` → `"success"`, no redirect fields set
- **Teardown**: Delete policy

### TC-POL-002: Create Policy with Action REDIRECT_TO_URL
- **Input**: `listener_id = 2567`, `name = "redirect-url-test"`, `action = "REDIRECT_TO_URL"`, `redirect_url = "https://example.com/redirect"`
- **Expected**: Policy created, `redirect_url` stored, `status = "success"`
- **Teardown**: Delete policy

### TC-POL-003: Create Policy with Action REDIRECT_TO_POOL
- **Input**: `listener_id = 2567`, `name = "redirect-pool-test"`, `action = "REDIRECT_TO_POOL"`, `redirect_pool_id = 72799`
- **Expected**: Policy created, pool resolved, `status = "success"`
- **Teardown**: Delete policy

### TC-POL-004: Create Policy with Priority
- **Input**: Same as TC-POL-002 + `priority = 100`
- **Expected**: Policy created with priority 100, evaluated before lower-priority policies
- **Teardown**: Delete policy

### TC-POL-005: Update Policy Name In-Place
- **Input**: TC-POL-002 created policy → update `name` to `"redirect-url-updated"`
- **Expected**: Name updated, `status = "pending"` → `"success"`, other fields unchanged
- **Teardown**: Delete policy

### TC-POL-006: Update Policy Action (REJECT → REDIRECT_TO_URL)
- **Input**: TC-POL-001 created policy → update `action = "REDIRECT_TO_URL"`, `redirect_url = "https://update.com"`
- **Expected**: Action and redirect updated in-place (replace modifier)
- **Teardown**: Delete policy

### TC-POL-007: Delete Policy
- **Input**: TC-POL-003 created policy
- **Action**: `terraform destroy`
- **Expected**: Policy removed from cloud, state cleared
- **Verification**: `viettelidc_ovpc_lb_l7_policies` no longer contains the policy

### TC-POL-008: Policy Name Regex Validation
- **Input**: `name = "invalid@name!"` with action REJECT
- **Expected**: Plan-time error: "must contain only alphanumeric characters, spaces, and . , @ _ -"

### TC-POL-009: REDIRECT_TO_URL Without redirect_url
- **Input**: `action = "REDIRECT_TO_URL"` without `redirect_url`
- **Expected**: Plan-time error: "redirect_url is required when action is REDIRECT_TO_URL"

### TC-POL-010: REDIRECT_TO_POOL Without redirect_pool_id
- **Input**: `action = "REDIRECT_TO_POOL"` without `redirect_pool_id`
- **Expected**: Plan-time error: "redirect_pool_id is required when action is REDIRECT_TO_POOL"

### TC-POL-011: REJECT With redirect_url Set
- **Input**: `action = "REJECT"`, `redirect_url = "https://should-not-be.com"`
- **Expected**: Plan-time error: "redirect_url must not be set when action is REJECT"

### TC-POL-012: Priority < 1
- **Input**: `action = "REJECT"`, `priority = 0`
- **Expected**: Plan-time error: "priority must be greater than or equal to 1"

---

## Group 2: L7 Rule CRUD

### TC-RUL-001: Create PATH Rule (STARTS_WITH)
- **Input**: `vpc_id = 40784`, `policy_id = 6`, `type = "PATH"`, `compare_type = "STARTS_WITH"`, `value = "/api/"`
- **Expected**: Rule created, `status = "pending"` → `"success"`, `listener_id` resolved from parent policy
- **Teardown**: Delete rule

### TC-RUL-002: Create PATH Rule (EQUAL_TO)
- **Input**: Same as TC-RUL-001, `compare_type = "EQUAL_TO"`, `value = "/login"`
- **Expected**: Rule matches exact path only
- **Teardown**: Delete rule

### TC-RUL-003: Create HOST_NAME Rule
- **Input**: `type = "HOST_NAME"`, `compare_type = "EQUAL_TO"`, `value = "app.example.com"`
- **Expected**: Rule matches exact hostname
- **Teardown**: Delete rule

### TC-RUL-004: Create HOST_NAME Rule (CONTAINS)
- **Input**: `type = "HOST_NAME"`, `compare_type = "CONTAINS"`, `value = "example.com"`
- **Expected**: Rule matches any hostname containing "example.com"
- **Teardown**: Delete rule

### TC-RUL-005: Create FILE_TYPE Rule (EQUAL_TO)
- **Input**: `type = "FILE_TYPE"`, `compare_type = "EQUAL_TO"`, `value = "jpg"`
- **Expected**: Rule matches ".jpg" extension
- **Teardown**: Delete rule

### TC-RUL-006: Create FILE_TYPE Rule (REGEX)
- **Input**: `type = "FILE_TYPE"`, `compare_type = "REGEX"`, `value = "(jpg|png|gif)"`
- **Expected**: Rule matches multiple extensions
- **Teardown**: Delete rule

### TC-RUL-007: Create FILE_TYPE Rule (Invalid Compare Type)
- **Input**: `type = "FILE_TYPE"`, `compare_type = "STARTS_WITH"`, `value = "jpg"`
- **Expected**: Plan-time error: "FILE_TYPE rule only supports EQUAL_TO or REGEX compare types"

### TC-RUL-008: Create HEADER Rule
- **Input**: `type = "HEADER"`, `compare_type = "EQUAL_TO"`, `key = "X-Custom-Header"`, `value = "test-value"`
- **Expected**: Rule matches exact header value
- **Teardown**: Delete rule

### TC-RUL-009: Create COOKIE Rule
- **Input**: `type = "COOKIE"`, `compare_type = "CONTAINS"`, `key = "session_id"`, `value = "abc"`
- **Expected**: Rule matches cookie containing "abc"
- **Teardown**: Delete rule

### TC-RUL-010: HEADER Without Key
- **Input**: `type = "HEADER"`, `key` omitted
- **Expected**: Plan-time error: "key is required for rule type HEADER"

### TC-RUL-011: COOKIE Without Key
- **Input**: `type = "COOKIE"`, `key` omitted
- **Expected**: Plan-time error: "key is required for rule type COOKIE"

### TC-RUL-012: PATH With Key (Forbidden)
- **Input**: `type = "PATH"`, `key = "should-not-be"`
- **Expected**: Plan-time error: "key must not be set for rule type PATH"

### TC-RUL-013: PATH Value Without Leading /
- **Input**: `type = "PATH"`, `value = "api/data"`
- **Expected**: Plan-time error: "PATH value must start with '/'"

### TC-RUL-014: FILE_TYPE Value With Leading Dot
- **Input**: `type = "FILE_TYPE"`, `value = ".jpg"`
- **Expected**: Plan-time error: "FILE_TYPE value must not start with '.'"

### TC-RUL-015: Key With Control Characters
- **Input**: `type = "HEADER"`, `key = "X-Test\x00Invalid"`
- **Expected**: Plan-time error: "key must not contain control characters"

### TC-RUL-016: Value > 255 Characters
- **Input**: `type = "PATH"`, `value = "/" + "a" * 255`
- **Expected**: Plan-time error: "value must not exceed 255 characters"

### TC-RUL-017: Key > 255 Characters
- **Input**: `type = "HEADER"`, `key = "X-" + "a" * 255`
- **Expected**: Plan-time error: "key must not exceed 255 characters"

### TC-RUL-018: Update Rule Value In-Place
- **Input**: TC-RUL-001 created rule → update `value = "/v2/"`
- **Expected**: Value updated, `status = "pending"` → `"success"`
- **Teardown**: Delete rule

### TC-RUL-019: Delete Rule
- **Input**: TC-RUL-003 created rule
- **Action**: `terraform destroy`
- **Expected**: Rule removed from cloud, state cleared

### TC-RUL-020: All 5 Compare Types on PATH
- **Inputs**: EQUAL_TO, STARTS_WITH, ENDS_WITH, CONTAINS, REGEX on `type = "PATH"`
- **Expected**: All succeed except REGEX may show warning if invalid regex
- **Teardown**: Delete each rule

---

## Group 3: Data Sources

### TC-DS-001: Query L7 Policies (viettelidc_ovpc_lb_l7_policies)
- **Input**: `vpc_id = 40784`, `listener_id = 2567`
- **Expected**: Returns list including existing policy ID 6 ("dev"), all attributes populated
- **Output attributes**: `policies[].id`, `policy_id`, `listener_id`, `name`, `action`, `redirect_url`, `redirect_pool_id`, `pool_name`, `priority`, `status`, `number_of_rules`

### TC-DS-002: Query L7 Policies Filter by Non-Existent Listener
- **Input**: `vpc_id = 40784`, `listener_id = 999999`
- **Expected**: Returns empty `policies` list (no error)

### TC-DS-003: Query L7 Rules (viettelidc_ovpc_lb_l7_rules)
- **Input**: `vpc_id = 40784`, `listener_id = 2567`, `policy_id = 6`
- **Expected**: Returns rules for policy 6, including TC-RUL-001 created rule after apply

### TC-DS-004: Query L7 Rules Without Policy Filter
- **Input**: `vpc_id = 40784`, `listener_id = 2567` (no `policy_id`)
- **Expected**: Returns all rules across all policies on listener 2567

### TC-DS-005: Query Available Pools (viettelidc_ovpc_lb_l7_available_pools)
- **Input**: `vpc_id = 40784`, `listener_id = 2567`
- **Expected**: Returns pool 72799 in list, with `id` and `name`

### TC-DS-006: Data Source Read After Rule Creation
- **Input**: Apply TC-RUL-001 → Read `viettelidc_ovpc_lb_l7_rules`
- **Expected**: New rule appears in list with correct `type`, `compare_type`, `value`

---

## Group 4: Listener Enhancements (vLB Layer 7)

### TC-LIS-001: HTTP to HTTPS Redirect
- **Prerequisite**: Listener with `protocol = "HTTP"`, `enable_redirect = true`, `redirect_domain`
- **Expected**: HTTP requests automatically redirect to HTTPS

### TC-LIS-002: Listener Domain Configuration
- **Input**: `domain = "api.example.com"` on HTTPS listener
- **Expected**: Domain associated with listener, SNI selects certificate

### TC-LIS-003: SNI Certificate Selection
- **Prerequisite**: Multiple certificates uploaded, listener with `protocol = "TERMINATED_HTTPS"`
- **Input**: `sni_certificates = [cert_id_1, cert_id_2]`
- **Expected**: Listener presents correct certificate based on SNI hostname

### TC-LIS-004: TERMINATED_HTTPS Without Certificate
- **Input**: `protocol = "TERMINATED_HTTPS"` without `certificate_id`
- **Expected**: Plan-time error (or API error if not caught at plan time)

### TC-LIS-005: HTTP Protocol with SNI Certificates
- **Input**: `protocol = "HTTP"` with `sni_certificates`
- **Expected**: Plan-time error or warning that SNI requires TERMINATED_HTTPS

---

## Group 5: Concurrency & Mutex

### TC-MTX-001: LockLB Serializes Operations
- **Setup**: Two `terraform apply` concurrently on same LB resources
- **Expected**: Second operation waits for first to complete (serialized by mutex)

### TC-MTX-002: Octavia 409 Prevention
- **Setup**: Rapid consecutive operations on same LB (create policy → create rule)
- **Expected**: No HTTP 409 Conflict errors due to mutex + settleDelay

### TC-MTX-003: SettleDelay Configuration
- **Setup**: `VTT_LB_SETTLE_DELAY = 5s` env var
- **Action**: Create policy → immediately create rule
- **Expected**: 5-second gap between operations, no race condition

### TC-MTX-004: Concurrent Policy+Rule Creation
- **Setup**: Two resources in same apply: `viettelidc_ovpc_lb_l7_policy` + `viettelidc_ovpc_lb_l7_rule`
- **Expected**: Rule waits for policy to settle, no 409

### TC-MTX-005: LB Settle Polling
- **Input**: Check `pollLBSettled` after each LB operation
- **Expected**: Waits for LB status = "ACTIVE" or "SUCCESS" before returning

---

## Group 6: Client Validation (Class A & B)

### TC-VAL-001: Class A - Action + Redirect URL Mismatch
- **Input**: `action = "REDIRECT_TO_URL"`, `redirect_url = ""`
- **Expected**: Plan-time error at validation stage

### TC-VAL-002: Class A - REDIRECT_TO_URL URL Format
- **Input**: `action = "REDIRECT_TO_URL"`, `redirect_url = "ftp://invalid.com"`
- **Expected**: Plan-time error: "redirect_url must start with http:// or https://"

### TC-VAL-003: Class A - REDIRECT_TO_URL With Spaces
- **Input**: `action = "REDIRECT_TO_URL"`, `redirect_url = "https://ex ample.com"`
- **Expected**: Plan-time error: "redirect_url must not contain spaces"

### TC-VAL-004: Class A - REDIRECT_TO_URL With Control Characters
- **Input**: `action = "REDIRECT_TO_URL"`, `redirect_url = "https://example.com\x00"`
- **Expected**: Plan-time error: "redirect_url must not contain control characters"

### TC-VAL-005: Class A - REDIRECT_TO_URL > 1024 Chars
- **Input**: `action = "REDIRECT_TO_URL"`, `redirect_url = "https://example.com/" + "a" * 1010`
- **Expected**: Plan-time error: "redirect_url length must not exceed 1024 characters"

### TC-VAL-006: Class B - HOST_NAME Pure Digits
- **Input**: `type = "HOST_NAME"`, `value = "12345"`
- **Expected**: Plan-time error: "HOST_NAME value must not consist solely of digits"

### TC-VAL-007: Class B - HOST_NAME With Protocol
- **Input**: `type = "HOST_NAME"`, `value = "https://example.com"`
- **Expected**: Plan-time error: "HOST_NAME value must not contain 'http://' or 'https://'"

### TC-VAL-008: Class B - PATH Regex Warning
- **Input**: `type = "PATH"`, `compare_type = "REGEX"`, `value = "[invalid"`
- **Expected**: Plan-time warning about invalid regex syntax (Go RE2)

### TC-VAL-009: Class B - Key Semicolon/Comma
- **Input**: `type = "HEADER"`, `key = "X-Test;value"`
- **Expected**: Plan-time error: "key must not contain semicolons, commas, equals, or quotes"

### TC-VAL-010: Class B - Cookie Key Semicolon
- **Input**: `type = "COOKIE"`, `key = "session;id"`
- **Expected**: Plan-time error (same as TC-VAL-009)

---

## Group 7: Cascade Deletion & Error Recovery

### TC-CAS-001: Delete Policy With Rules
- **Setup**: Policy with existing rules (e.g., policy 6)
- **Action**: `terraform destroy` on policy resource
- **Expected**: All rules deleted first (or cascade delete), policy removed

### TC-CAS-002: Delete Rule Only
- **Setup**: Policy with rules
- **Action**: `terraform destroy` on specific rule resource
- **Expected**: Only that rule deleted, policy and other rules intact

### TC-CAS-003: Orphan Rule Detection
- **Setup**: Rule created via API, not in Terraform state
- **Action**: `terraform apply` with new rule resource for same policy
- **Expected**: No duplicate rules; backend reconciliation

### TC-CAS-004: Error Recovery - Policy Creation Fails
- **Setup**: Invalid redirect_pool_id (non-existent)
- **Action**: `terraform apply`
- **Expected**: Error message, no resource created in state

### TC-CAS-005: Error Recovery - Rule Creation Fails
- **Setup**: Invalid compare_type for rule type
- **Action**: `terraform apply`
- **Expected**: Plan-time error prevents creation attempt

### TC-CAS-006: Import Existing Policy
- **Input**: `terraform import viettelidc_ovpc_lb_l7_policy.example 40784/2567/6`
- **Expected**: State populated with existing policy 6 attributes

### TC-CAS-007: Import Existing Rule
- **Input**: After TC-RUL-001 applied: `terraform import viettelidc_ovpc_lb_l7_rule.example 40784/6/<rule_id>`
- **Expected**: State populated with existing rule attributes

### TC-CAS-008: Delete Non-Existent Resource
- **Action**: `terraform destroy` when resource already gone from cloud
- **Expected**: No error; graceful handling of 404

### TC-CAS-009: Concurrent Delete
- **Setup**: Rule resource + data source query simultaneously
- **Action**: Delete rule while data source reads
- **Expected**: No panic; empty result or consistent state

---

## Test Execution Matrix

| Group | Test Count | Manual? | Automated? |
|-------|------------|---------|------------|
| Group 1: Policy CRUD | 12 | TC-POL-008 to TC-POL-012 | TC-POL-001 to TC-POL-007 |
| Group 2: Rule CRUD | 20 | TC-RUL-007, TC-RUL-010 to TC-RUL-017 | TC-RUL-001 to TC-RUL-006, TC-RUL-018 to TC-RUL-020 |
| Group 3: Data Sources | 6 | — | All |
| Group 4: Listener Enhancements | 5 | TC-LIS-001 to TC-LIS-003 | TC-LIS-004, TC-LIS-005 |
| Group 5: Concurrency & Mutex | 5 | TC-MTX-001 | TC-MTX-002 to TC-MTX-005 |
| Group 6: Validation | 10 | — | All |
| Group 7: Cascade & Recovery | 9 | TC-CAS-001 | TC-CAS-002 to TC-CAS-009 |
| **Total** | **67** | **16** | **51** |

---

## Quick Smoke Test (Live Cloud)

For live cloud verification with the provided credentials:

```bash
cd _smoke/tf-real/ovpc-subnet-example

# Query existing policies
terraform plan -var="vpc_id=40784" -var="listener_id=2567"

# Create PATH rule on policy 6
terraform apply -auto-approve

# Verify rules
terraform apply -refresh-only
```
