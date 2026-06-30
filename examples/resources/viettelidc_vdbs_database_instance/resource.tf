terraform {
  required_providers {
    viettelidc = {
      source = "viettelidc/viettelidc"
    }
  }
}

# The viettelidc_vdbs_database_instance resource does not support creation via Terraform. 
# It must be created via the web console and then imported using 'terraform import'.
# Example usage after import:
resource "viettelidc_vdbs_database_instance" "example" {
  # The ID of the database instance (uuid)
  id = "12345678-1234-1234-1234-123456789012"

  # The required admin password
  admin_password = "MySuperSecretPassword123!"

  # Manage the state (RUNNING or STOPPED)
  desired_state = "RUNNING"
}
