---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_certificate (Data Source)"
sidebar_current: "docs-viettelidc-datasource-certificate"
description: |-
  Retrieves information about an existing ViettelIDC TLS/SSL Certificate.
---

# Data Source: viettelidc\_ovpc\_certificate

Use this data source to look up a TLS/SSL Certificate by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_ovpc_certificate" "tls" {
  name   = "my-tls-cert"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Certificate UUID. Either `id` or `name` must be set.
* `name` - (Optional) Certificate name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Certificate UUID.
* `name` - Name.
* `status` - Status.
* `created_at` - Creation timestamp.
