resource "viettelidc_ovpc_launch_template" "web" {
  name        = "web-template"
  description = "Template for web servers"
  vm_id       = viettelidc_ovpc_instance.base_vm.id
  memory_size = 4
  cpu_size    = 2
  vpc_id      = viettelidc_ovpc_vpc.main.id
}
