# Example Usage
data "viettelidc_voks_addon_versions" "addon_versions" {
  name               = "dashboard"
  kubernetes_version = "1.10.1"
  filter {
    version = "v1.10.1-eksbuild.1"
  }

  #Attribute Reference
  versions = [
    "v1.10.1-eksbuild.1",
    "v1.10.1-eksbuild.2"
  ]
}