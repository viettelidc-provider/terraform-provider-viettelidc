---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "viettelidc_voks_cluster Resource - viettelidc"
subcategory: ""
description: |-
  Manage a cluster resource within ViettelIdc.
---

# viettelidc_voks_cluster (Resource)

Manage a cluster resource within ViettelIdc.

## Example Usage

```terraform
# Example Usage - with cluster
resource "viettelidc_voks_cluster" "example" {
  name    = "k8s-cluster"
  version = "1.8.0"

  vpc_config {
    vpc_id = "234134"
  }
}


# Example Usage - with NFS
resource "viettelidc_voks_cluster" "example" {
  name    = "k8s-cluster"
  version = "1.8.0"

  vpc_config {
    vpc_id = "234134"
  }

  nfs = {
    additional_storage_size = 20
  }
}


# Example Usage - with node group
resource "viettelidc_voks_cluster" "example" {
  name    = "k8s-cluster"
  version = "1.8.0"

  vpc_config {
    vpc_id = "234134"
  }

  node_group {
    resource_type = "T1.vOKS 1"
    scaling_config = {
      enable_auto_scale = false
      min_node          = 1
      max_node          = 2
    }

    auto_repair = false

    labels = {
      environment = "production"
      team        = "devops"
      app         = "backend"
      region      = "us-west"
    }

    taint {
      key    = "dedicated"
      value  = "gpu"
      effect = "NO_SCHEDULE"
    }
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `name` (String) Name of the VettelIdc Kuberneters Cluster.
- `version` (String) Version of ViettelIdc Kubernetes Cluster.

### Optional

- `nfs` (Attributes) NFS storage enables multiple nodes in the cluster to access the same file system over a network. (see [below for nested schema](#nestedatt--nfs))
- `vpc_config` (Block, Optional) The vpc_config is a configuration that helps define the networking setup for the ViettelIdc Kubernetes Cluster. (see [below for nested schema](#nestedblock--vpc_config))

### Read-Only

- `endpoint` (String) Endpoint for your ViettelIdc Kuberneters API server.
- `id` (Number) Id of the VettelIdc Kuberneters Cluster.
- `status` (String) The current status of ViettelIdc Kubernetes Cluster. Valid values: `POWER_ON`, `POWER_OFF`, `ERROR`.

<a id="nestedatt--nfs"></a>
### Nested Schema for `nfs`

Optional:

- `additional_storage_size` (Number) The additional storage allocated for NFS volumes.

Read-Only:

- `cpu` (Number) The CPU size of NFS server.
- `ip_address` (String) Internal IP of NFS server that can be accessed by internal network of your ViettelIdc Kubernetes Cluster.
- `memory` (Number) The memory size of NFS server.
- `status` (String) Status of ViettelIdc Kubernetes Cluster NFS Storage. When the NFS Storage is present in Terraform, its status will always be `POWER_ON`. Valid values: `POWER_ON`, `UPDATING`, `ERROR`.
- `total_storage_size` (Number) The size allocated for NFS volumes.


<a id="nestedblock--vpc_config"></a>
### Nested Schema for `vpc_config`

Required:

- `vpc_id` (Number) ID of the VPC associated with your cluster.
