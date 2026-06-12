---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_vm_templates (Data Source)"
sidebar_current: "docs-viettelidc-datasource-vm-templates"
description: |-
  Lists available VM OS/flavor templates in a ViettelIDC VPC.
---

# Data Source: viettelidc\_ovpc\_vm\_templates

Use this data source to list available VM OS/flavor templates. The `id` of each template is used as `template_id` when creating a `viettelidc_ovpc_instance`.

## Example Usage

```hcl
# List all templates whose name contains "Ubuntu 22.04"
data "viettelidc_ovpc_vm_templates" "ubuntu" {
  name_filter = "Ubuntu 22.04"
  host_id     = 6
}

# Use the first matching template when creating an instance
resource "viettelidc_ovpc_instance" "vm1" {
  template_id = data.viettelidc_ovpc_vm_templates.ubuntu.templates[0].id
  # ...
}
```

## Argument Reference

* `name_filter` - (Optional) Partial name filter (e.g. `"ubun"` matches Ubuntu templates).
* `page_size` - (Optional) Maximum results to return. Defaults to `100`.
* `host_id` - (Optional) Host ID (hypervisor cluster ID) (e.g. `6`). Some API endpoints require this parameter to return results.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `vpc_id` - Resolved VPC ID used for the query.
* `templates` - List of matching templates. Each item exports:
  * `id` - Template ID — use as `template_id` in `viettelidc_ovpc_instance`.
  * `name` - Template name (e.g. `Ubuntu 22.04`).
  * `description` - Template description.
  * `os_type` - OS type (e.g. `Linux`, `Windows`).
  * `cpu` - Number of vCPUs.
  * `memory` - Memory in MB.
