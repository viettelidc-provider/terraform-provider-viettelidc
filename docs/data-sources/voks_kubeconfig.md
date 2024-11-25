---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "viettelidc_voks_kubeconfig Data Source - viettelidc"
subcategory: ""
description: |-
  The configuration for accessing the cluster.
---

# viettelidc_voks_kubeconfig (Data Source)

The configuration for accessing the cluster.

## Example Usage

```terraform
data "viettelidc_voks_kubeconfig" "example" {
  clusterId = 456
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `cluster_id` (Number) Id of the Viettel Kuberneters Cluster.

### Read-Only

- `value` (String) The kubeconfig file is essential for configuring access to the cluster, providing connection details, authentication credentials, and other configurations.