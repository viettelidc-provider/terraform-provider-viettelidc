terraform {
  required_providers {
    viettelidc = {
      source = "viettelidc/viettelidc"
    }
  }
}

# The viettelidc_vks_cluster resource does not support creation via Terraform. 
# It must be created via the web console and then imported using 'terraform import'.
# Example usage after import:
resource "viettelidc_vks_cluster" "example" {
  # The ID of the kubernetes cluster (uuid)
  id = "12345678-1234-1234-1234-123456789012"

  # Upgrade the version of the cluster
  version = "1.28"
}
