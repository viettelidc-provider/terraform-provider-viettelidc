# Example Usage
data "viettelidc_voks_addon" "addon" {
  cluster_id    = "1234"
  name          = "coredns"
  addon_version = "v1.10.1-eksbuild.1"
  status        = "SUCCESS"
}
