---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_key_pair"
sidebar_current: "docs-viettelidc-resource-key-pair"
description: |-
  Provides a ViettelIDC SSH Key Pair.
---

# viettelidc\_ovpc\_key\_pair

Provides an SSH Key Pair for VM Instance access on ViettelIDC.

~> **Note:** `download_url` contains the URL to download the private key. The URL is only valid immediately after creation. Save the value right away and keep it secure.

## Example Usage

```hcl
resource "viettelidc_ovpc_key_pair" "deploy" {
  key_name = "deploy-key"
  vpc_id   = viettelidc_ovpc_vpc.main.id
}

output "private_key_url" {
  value     = viettelidc_ovpc_key_pair.deploy.download_url
  sensitive = true
}
```

## Argument Reference

* `key_name` - (Required, ForceNew) Key pair name. **Cannot be changed after creation.**
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Key Pair ID.
* `download_url` - (Sensitive) URL to download the private key. Only available immediately after creation.

## Import

Key Pairs can be imported using the Key Pair ID:

```
terraform import viettelidc_ovpc_key_pair.deploy <key_pair_id>
```
