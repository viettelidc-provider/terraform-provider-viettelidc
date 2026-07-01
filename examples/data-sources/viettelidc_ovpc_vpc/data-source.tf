data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

output "vpc_cidr" {
  value = data.viettelidc_ovpc_vpc.main.cidr_block
}
