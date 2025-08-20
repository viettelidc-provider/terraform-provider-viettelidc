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

# Fetch information about a specific VPC
data "viettelidc_vpc" "example" {
  vpc_id = 12345
  id     = 67890
}

# Output the VPC information
output "vpc_info" {
  value = {
    id     = data.viettelidc_vpc.example.id
    name   = data.viettelidc_vpc.example.name
    status = data.viettelidc_vpc.example.status
    tierid = data.viettelidc_vpc.example.tierid
  }
}
