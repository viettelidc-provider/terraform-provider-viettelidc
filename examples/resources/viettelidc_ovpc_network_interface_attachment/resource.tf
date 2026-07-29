resource "viettelidc_ovpc_network_interface_attachment" "attach" {
  network_interface_id = viettelidc_ovpc_network_interface.nic.id
  instance_id          = viettelidc_ovpc_instance.vm.id
  vpc_id               = data.viettelidc_ovpc_vpc.main.id
}
