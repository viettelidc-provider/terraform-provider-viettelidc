---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_certificate"
sidebar_current: "docs-viettelidc-resource-certificate"
description: |-
  Provides a ViettelIDC TLS/SSL Certificate managed by the key-manager service.
---

# viettelidc\_ovpc\_certificate

Provides a TLS/SSL Certificate stored in the ViettelIDC key-manager service. Certificates are commonly used with Load Balancers for HTTPS termination.

~> **Note:** `certificate` and `private_key` are immutable — changing either field forces a new resource.

## Example Usage

```hcl
resource "viettelidc_ovpc_certificate" "tls" {
  name        = "my-tls-cert"
  vpc_id      = viettelidc_ovpc_vpc.main.id
  certificate = file("cert.pem")
  private_key = file("key.pem")
}
```

## Argument Reference

* `name` - (Required) Display name of the certificate.
* `certificate` - (Required, Sensitive, ForceNew) PEM-encoded certificate content (`-----BEGIN CERTIFICATE-----`).
* `private_key` - (Required, Sensitive, ForceNew) PEM-encoded private key (`-----BEGIN RSA PRIVATE KEY-----` or `-----BEGIN PRIVATE KEY-----`).
* `vpc_id` - (Optional, ForceNew) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Certificate UUID assigned by key-manager.
* `status` - Certificate status.
* `created_at` - Timestamp when the certificate was created.

## Import

Certificates can be imported using the Certificate ID:

```
terraform import viettelidc_ovpc_certificate.tls <certificate_id>
```
