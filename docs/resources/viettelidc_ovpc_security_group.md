---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_security_group"
sidebar_current: "docs-viettelidc-resource-security-group"
description: |-
  Provides a ViettelIDC Security Group.
---

# viettelidc\_ovpc\_security\_group

Provides a Security Group resource — a named firewall container that holds inbound and outbound rules. Use `viettelidc_ovpc_security_group_rule` to add rules.

## Example Usage

```hcl
resource "viettelidc_ovpc_security_group" "web" {
  name        = "web-sg"
  description = "Security group for web servers"
  vpc_id      = viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_security_group_rule" "allow_http" {
  security_group_id = viettelidc_ovpc_security_group.web.id
  direction         = "in"
  rule_type         = "HTTP"
  source_ip         = "0.0.0.0/0"
  vpc_id            = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required) Security Group name.
* `description` - (Optional) Description.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Security Group ID (`vttSecurityGroupId`).

## Import

Security Groups can be imported using the Security Group ID:

```
terraform import viettelidc_ovpc_security_group.web <security_group_id>
```
