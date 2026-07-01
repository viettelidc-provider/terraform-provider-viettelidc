resource "viettelidc_ovpc_floating_ip" "fip" {
  instance_id          = viettelidc_ovpc_instance.vm.id
  network_interface_id = viettelidc_ovpc_instance.vm.root_nic_id
  vpc_id               = viettelidc_ovpc_vpc.main.id
}
