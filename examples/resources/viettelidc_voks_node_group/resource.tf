# Example Usage - basic
resource "viettelidc_voks_node_group" "example" {
  cluster_id    = 123
  name          = "k8s-node-group"
  resource_type = "T1.vOKS 1"

  scaling_config {
    enable_auto_scale = true
    min_node          = 1
    max_node          = 2
  }
}

# Example Usage - with auto repair
resource "viettelidc_voks_node_group" "example" {
  cluster_id    = 123
  name          = "k8s-node-group"
  resource_type = "T1.vOKS 1"

  scaling_config {
    enable_auto_scale = true
    min_node          = 1
    max_node          = 2
  }

  auto_repair = false
}

# Example Usage - with lables
resource "viettelidc_voks_node_group" "example" {
  cluster_id    = 123
  name          = "k8s-node-group"
  resource_type = "T1.vOKS 1"

  scaling_config {
    enable_auto_scale = true
    min_node          = 1
    max_node          = 2
  }

  labels = {
    environment = "production"
    team        = "devops"
    app         = "backend"
    region      = "us-west"
  }
}

# Example Usage - with taint
resource "viettelidc_voks_node_group" "example" {
  cluster_id    = 123
  name          = "k8s-node-group"
  resource_type = "T1.vOKS 1"

  scaling_config {
    enable_auto_scale = true
    min_node          = 1
    max_node          = 2
  }

  taint {
    key    = "dedicated"
    value  = "gpu"
    effect = "NoSchedule"
  }
}

# Example Usage - with scaling config, labels and taint
resource "viettelidc_voks_node_group" "example" {
  cluster_id    = 123
  name          = "k8s-node-group"
  resource_type = "T1.vOKS 1"

  scaling_config {
    enable_auto_scale = true
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
    effect = "NoSchedule"
  }
}