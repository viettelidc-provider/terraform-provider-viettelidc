---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "viettelidc_voks_cluster Data Source - viettelidc"
subcategory: ""
description: |-
  Retrieve information about a vOKS Cluster
---

# viettelidc_voks_cluster (Data Source)

Retrieve information about a vOKS Cluster

## Example Usage

```terraform
# Example Usage
data "viettelidc_voks_cluster" "cluster" {

  id = "56231"

  #Attribute Reference
  name     = "k8s-cluster"
  status   = "SUCCESS"
  version  = "v1.29.8"
  endpoint = "https://172.17.11.53:6443"
  nfs = {
    cpu                = "2"
    memory             = "2"
    total_storage_size = "100"
    status             = "POWERED_ON"
    ip_address         = "10.20.29.230"
  }
  vpc_config = {
    security_group_ids = [11900, 11909, 11912]
    subnet_ids         = [4379, 4385]
    vpc_id             = 19490
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `id` (Number) Id of the Cluster.

### Optional

- `nfs` (Attributes) NFS storage enables multiple nodes in the cluster to access the same file system over a network. (see [below for nested schema](#nestedatt--nfs))

### Read-Only

- `endpoint` (String) Endpoint is IP address and port number that define the backend pod associated with a vOKS service.
- `name` (String) Name of the Cluster.
- `status` (String) The current status of Cluster. Valid values: `POWER_ON`, `POWER_OFF`, `ERROR`.
- `version` (String) Version of Cluster.
- `vpc_config` (Block, Read-only) The vpc_config is a configuration that helps define the networking setup for the ViettelIdc Kubernetes Cluster. (see [below for nested schema](#nestedblock--vpc_config))

<a id="nestedatt--nfs"></a>
### Nested Schema for `nfs`

Read-Only:

- `cpu` (Number) The CPU size of NFS server.
- `ip_address` (String) Internal IP of NFS server that can be accessed by internal network of your Cluster.
- `memory` (Number) The memory size of NFS server.
- `status` (String) Status of Cluster NFS Storage. When the NFS Storage is present in Terraform, its status will always be `POWER_ON`. Valid values: `POWER_ON`, `UPDATING`, `ERROR`.
- `total_storage_size` (Number) The size allocated for NFS volumes.


<a id="nestedblock--vpc_config"></a>
### Nested Schema for `vpc_config`

Read-Only:

- `security_group_ids` (List of Number) The IDs of the security group to be associated with the VPC endpoint.
- `subnet_ids` (List of Number) The IDs of the subnets to be associated with the VPC endpoint.
- `vpc_id` (Number) ID of the VPC associated with your cluster.