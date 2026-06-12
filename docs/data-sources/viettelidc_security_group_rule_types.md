---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_security_group_rule_types"
sidebar_current: "docs-viettelidc-datasource-security-group-rule-types"
description: |-
  Lists all available Security Group rule types.
---

# viettelidc\_security\_group\_rule\_types

Lists all available Security Group rule types supported by the platform. Use the `name` of each item as the `rule_type` argument in `viettelidc_ovpc_security_group_rule`.

## Example Usage

```hcl
data "viettelidc_ovpc_security_group_rule_types" "all" {}

output "rule_type_names" {
  value = [for t in data.viettelidc_ovpc_security_group_rule_types.all.rule_types : t.name]
}
```

## Argument Reference

* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `rule_types` - List of rule type objects with the following attributes:
  * `id` - Internal numeric ID of the rule type.
  * `name` - Rule type name. Use this as `rule_type` in `viettelidc_ovpc_security_group_rule`.
  * `default_port` - Default port for this rule type (e.g. `"22"`, `"80"`, `"Any"`).
  * `default_protocol` - Internal protocol ID.
  * `port_enabled` - Whether a custom port can be specified (`true` only for `"Custom TCP"` and `"Custom UDP"`).
  * `protocol_enabled` - Whether a custom protocol can be specified.

## Known Rule Types

The following rule types are available on the platform (as of 2026-06):

| Name | Default Port | Requires `port` |
|------|-------------|-----------------|
| `Custom TCP` | — | Yes |
| `Custom UDP` | — | Yes |
| `All TCP` | Any | No |
| `All UDP` | Any | No |
| `All ICMP - IPv4` | Any | No |
| `SSH` | 22 | No |
| `DNS` | 53 | No |
| `HTTP` | 80 | No |
| `HTTPS` | 443 | No |
| `IMAP` | 143 | No |
| `IMAPS` | 993 | No |
| `LDAP` | 389 | No |
| `MS SQL` | 1433 | No |
| `MYSQL` | 3306 | No |
| `POP3` | 110 | No |
| `POP3S` | 995 | No |
| `RDP` | 3389 | No |
| `SMTP` | 25 | No |
| `SMTPS` | 465 | No |
| `Other Protocol - IPv4` | Any | No |
