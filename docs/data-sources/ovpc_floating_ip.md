---
page_title: "viettelidc_ovpc_floating_ip Data Source - viettelidc"
subcategory: "Virtual Private Cloud (OVPC)"
description: |-
  Look up a ViettelIDC Floating IP by its Public IP address or ID.
---

# viettelidc_ovpc_floating_ip (Data Source)

Look up a ViettelIDC Floating IP by its Public IP address or ID.

## Example Usage

```terraform
data "viettelidc_ovpc_floating_ip" "fip" {
  public_ip = "1.2.3.4"
  vpc_id    = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

The following arguments are supported:

- `public_ip` (Optional) - Public IPv4 address to search for. Mutually optional with `id`.
- `id` (Optional) - Floating IP ID (vttFloatingId) to search for. Mutually optional with `public_ip`.
- `vpc_id` (Optional) - VPC ID. Defaults to the provider's default `vpc_id` if not set.

## Attributes Reference

In addition to all arguments above, the following attributes are exported (can be accessed via `data.viettelidc_ovpc_floating_ip.<name>.<attribute>`):

- `id` - The ID of the Floating IP.
- `public_ip` - The Public IPv4 address.
- `instance_id` - The ID of the VM instance it is associated with.
- `network_interface_id` - The ID of the NIC it is associated with.
- `name` - The name of the Floating IP.
- `type` - The type of the Floating IP (e.g., public).
- `status` - The status of the Floating IP.
- `attachment_status` - The attachment status (e.g., AVAILABLE).
