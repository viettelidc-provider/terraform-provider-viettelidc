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
