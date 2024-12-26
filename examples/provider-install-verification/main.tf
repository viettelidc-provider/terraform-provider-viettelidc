terraform {
  required_providers {
    viettelidc = {
      source = "hashicorp.com/edu/viettelidc"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.5.2"
    }
  }
}

# variable "mfa_code" {
#   type = string
# }

provider "local" {}

# Credentials are read from VIETTELIDC_DOMAIN_ID, VIETTELIDC_USERNAME and
# VIETTELIDC_PASSWORD.
provider "viettelidc" {}

data "viettelidc_voks_cluster" "cluster" {
  id = 2555
}

output "cluster" {
  value = data.viettelidc_voks_cluster.cluster
}

data "viettelidc_voks_addons" "addons" {
  kubernetes_version = "v1.29.8"
  filter = {
    name = "dashboard"
  }
}
output "addons" {
  value = data.viettelidc_voks_addons.addons
}

data "viettelidc_voks_addon_versions" "versions" {
  name               = data.viettelidc_voks_addons.addons.names[0]
  kubernetes_version = "v1.29.8"
}
output "versions" {
  value = data.viettelidc_voks_addon_versions.versions
}

data "viettelidc_voks_addon" "addon" {
  cluster_id = 2555
  name       = "dashboard"
}
output "addon" {
  value = data.viettelidc_voks_addon.addon
}

# data "viettelidc_voks_node_group" "node_group" {
#   id = 2186
#   cluster_id = 2555
# }
# output "node_group" {
#   value = data.viettelidc_voks_node_group.node_group
# }

# resource "local_file" "kubeconfig" {
#   content  = data.viettelidc_voks_kubeconfig.kube.value
#   filename = "kubeconfig.yml"
# }
#
# resource "viettelidc_voks_addon" "dashboard" {
#   cluster_id = "2483"
#   name       = "dashboard"
#   version    = "6.0.8"
# }
#
# resource "viettelidc_voks_node_group" "testing" {
#
#   cluster_id    = 2483
#   name          = "iac-testing"
#   resource_type = "T1.vOKS 1"
#
#   scaling_config {
#     enable_auto_scale = true
#     min_node          = 1
#     max_node          = 3
#   }
#
#   auto_repair = true
#
#   labels = {
#     environment = "production"
#     team        = "devops"
#     app         = "backend"
#     region      = "us-west"
#   }
#
#   taint {
#     key    = "dedicated"
#     value  = "gpu"
#     effect = "NoSchedule"
#   }
#
#   taint {
#     key    = "testing"
#     value  = "testing"
#     effect = "NoSchedule"
#   }
# }
#
# resource "viettelidc_voks_cluster" "example" {
#   name    = "k8s-idc-e08e222e"
#   version = "v1.28.4"
#   vpc_config {
#     vpc_id = "19205"
#   }
#   nfs = {
#     additional_storage_size = 10
#   }
# }