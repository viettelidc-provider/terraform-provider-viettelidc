---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_security_group_rule"
sidebar_current: "docs-viettelidc-resource-security-group-rule"
description: |-
  Provides a ViettelIDC Security Group Rule.
---

# viettelidc\_ovpc\_security\_group\_rule

Provides an inbound or outbound rule for a Security Group on ViettelIDC. All attributes are immutable — any change forces a new resource.

~> **Note:** The API has no separate delete endpoint for rules. Destroying this resource calls the update endpoint with `action = "Delete"`.

## Example Usage

```hcl
# Inbound SSH rule
resource "viettelidc_ovpc_security_group_rule" "ssh" {
  security_group_id = viettelidc_ovpc_security_group.web.id
  direction         = "in"
  rule_type         = "SSH"
  source_ip         = "0.0.0.0/0"
  vpc_id            = viettelidc_ovpc_vpc.main.id
}

# Custom inbound TCP rule
resource "viettelidc_ovpc_security_group_rule" "app" {
  security_group_id = viettelidc_ovpc_security_group.web.id
  direction         = "in"
  rule_type         = "Custom TCP"
  port              = "8080"
  source_ip         = "10.0.0.0/8"
  vpc_id            = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `security_group_id` - (Required, ForceNew) ID of the Security Group.
* `direction` - (Required, ForceNew) Traffic direction: `"in"` (inbound) or `"out"` (outbound).
* `rule_type` - (Required, ForceNew) Rule type name. Use `data.viettelidc_ovpc_security_group_rule_types` to query all available values at runtime. Known values: `"Custom TCP"`, `"Custom UDP"`, `"All TCP"`, `"All UDP"`, `"All ICMP - IPv4"`, `"SSH"`, `"DNS"`, `"HTTP"`, `"HTTPS"`, `"IMAP"`, `"IMAPS"`, `"LDAP"`, `"MS SQL"`, `"MYSQL"`, `"POP3"`, `"POP3S"`, `"RDP"`, `"SMTP"`, `"SMTPS"`, `"Other Protocol - IPv4"`.
* `protocol_name` - (Optional, Computed, ForceNew) Protocol: `"TCP"`, `"UDP"`, or `"ICMP"`. Auto-derived from `rule_type` if not set.
* `port` - (Optional, Computed, ForceNew) Port or port range (e.g. `"22"`, `"8000-9000"`). Required when `rule_type` is `"Custom TCP"` or `"Custom UDP"`.
* `source_ip` - (Optional, Computed, ForceNew) Source CIDR for inbound rules (e.g. `"0.0.0.0/0"`).
* `destination_ip` - (Optional, Computed, ForceNew) Destination CIDR for outbound rules.
* `action` - (Optional, Computed) Internal API action. **Do not set manually** — provider uses `"New"` on create and `"Delete"` on destroy automatically.
* `is_valid` - (Optional, Computed) Whether the rule is valid.
* `vpc_id` - (Optional, Computed) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Rule ID.

## Import

Security Group Rules can be imported using `<security_group_id>/<rule_id>`:

```
terraform import viettelidc_ovpc_security_group_rule.ssh <security_group_id>/<rule_id>
```
