# Example Usage
data "viettelidc_voks_addon" "addon" {
  cluster_id = "1234"
  name       = "coredns"
}

# Attribute Reference - these are read-only; do not set them in the block above.
output "addon_version" {
  value = data.viettelidc_voks_addon.addon.version
}

output "addon_status" {
  value = data.viettelidc_voks_addon.addon.status
}
