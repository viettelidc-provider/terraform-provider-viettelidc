resource "viettelidc_ovpc_vpc" "main" {
  name        = "my-vpc"
  cidr_block  = "10.0.0.0/16"
  description = "Main project VPC"
}
