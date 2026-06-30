terraform {
  required_providers {
    viettelidc = {
      source = "viettelidc/viettelidc"
    }
  }
}

# The viettelidc_vks_nfs_storage resource does not support creation via Terraform. 
# It must be created via the web console and then imported using 'terraform import'.
# Example usage after import:
resource "viettelidc_vks_nfs_storage" "example" {
  cluster_id   = "cluster-123456"
  storage_size = 100
}
