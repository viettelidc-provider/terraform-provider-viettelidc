# Example Usage
data "viettelidc_voks_node_group" "example" {
  cluster_id    = 123
  name          = "k8s-node-group"
  resource_type = "T1.vOKS 1"
  auto_repair   = false
  labels = {
    environment = "production"
    team        = "devops"
    app         = "backend"
    region      = "us-west"
  }
  status = "SUCCESS"
}
