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

# Fetch list of VPCs
data "viettelidc_vpcs" "example" {
  vpc_id  = 12345
  host_id = 67890
}

# Output the VPC list
output "vpc_list" {
  value = data.viettelidc_vpcs.example.items
}

# Output specific VPC information
output "first_vpc" {
  value = length(data.viettelidc_vpcs.example.items) > 0 ? {
    id     = data.viettelidc_vpcs.example.items[0].id
    name   = data.viettelidc_vpcs.example.items[0].name
    status = data.viettelidc_vpcs.example.items[0].status
    tierid = data.viettelidc_vpcs.example.items[0].tierid
  } : null
}
