# Every load balancer in the VPC, for when the name is not known up front.
data "viettelidc_ovpc_load_balancers" "all" {
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

output "internal_lb_addresses" {
  value = {
    for lb in data.viettelidc_ovpc_load_balancers.all.load_balancers :
    lb.name => lb.ip_address if !lb.is_public_loadbalancer
  }
}
