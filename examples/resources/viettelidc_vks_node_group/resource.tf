terraform {
  required_providers {
    viettelidc = {
      source = "viettelidc/viettelidc"
    }
  }
}

# The viettelidc_vks_node_group resource does not support creation via Terraform. 
# It must be created via the web console and then imported using 'terraform import'.
# Example usage after import:
resource "viettelidc_vks_node_group" "example" {
  cluster_id = "cluster-123456"

  # Note: viettelidc_vks_node_group does not support Create, Update or Delete via Terraform.
  # All properties except cluster_id are read-only (Computed).
  # Use this resource strictly for import-only state management.
}
