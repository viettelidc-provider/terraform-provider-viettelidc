# Example Usage
data "viettelidc_voks_node_group" "example" {

  id         = 123
  cluster_id = 123

  #Attribute Reference
  name          = "k8s-node-group"
  resource_type = "T1.vOKS 1"
  status        = "SUCCESS"

  auto_repair = false
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

  taint {
    key    = "dedicated"
    value  = "gpu"
    effect = "NoSchedule"
  }
}
