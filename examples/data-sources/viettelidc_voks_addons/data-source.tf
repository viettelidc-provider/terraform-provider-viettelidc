# Example Usage
data "viettelidc_voks_addons" "addons" {
  kubernetes_version = "1.10.1"
  filter = {
    name = "dashboard"
  }

  #Attribute Reference
  names = [
    "coredns",
    "dashboard"
  ]
}