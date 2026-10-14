data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

data "viettelidc_ovpc_internet_gateways" "all" {
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

output "default_internet_gateway_id" {
  value = data.viettelidc_ovpc_internet_gateways.all.internet_gateways[0].id
}
