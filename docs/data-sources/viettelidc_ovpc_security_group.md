---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_security_group (Data Source)"
sidebar_current: "docs-viettelidc-datasource-security-group"
description: |-
  Retrieves information about an existing ViettelIDC Security Group.
---

# Data Source: viettelidc\_ovpc\_security\_group

Use this data source to look up a Security Group by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_ovpc_security_group" "default" {
  name   = "default-sg"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Security Group ID. Either `id` or `name` must be set.
* `name` - (Optional) Security Group name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Security Group ID.
* `name` - Name.
* `description` - Description.
