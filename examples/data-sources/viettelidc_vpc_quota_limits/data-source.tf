terraform {
  required_providers {
    viettelidc = {
      source = "viettelidc-provider/viettelidc"
    }
  }
}

# Configure the ViettelIDC Provider
provider "viettelidc" {
  # Configuration options
}

# Fetch VPC quota limits
data "viettelidc_vpc_quota_limits" "example" {
  vpc_id = 12345
}

# Output the quota limits
output "quota_limits" {
  value = data.viettelidc_vpc_quota_limits.example.items
}

