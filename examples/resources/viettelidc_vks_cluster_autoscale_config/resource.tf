# Cáº¥u hÃ¬nh Autoscale Config cho Cluster
resource "viettelidc_vks_cluster_autoscale_config" "main" {
  cluster_id                       = "1477"
  scale_down_delay_after_add       = "300"
  scale_down_delay_after_delete    = "2"
  scale_down_delay_after_failure   = "120"
  scale_down_unneeded_time         = "120"
  scale_down_utilization_threshold = "0.1"
  scan_interval                    = "10"
}
