# 1. Allocate a new Floating IP and associate to an instance
resource "viettelidc_ovpc_floating_ip" "fip" {
  instance_id          = viettelidc_ovpc_instance.vm.id
  network_interface_id = viettelidc_ovpc_instance.vm.root_nic_id
  vpc_id               = data.viettelidc_ovpc_vpc.main.id
}

# 2. Associate an existing Floating IP directly to a Network Interface by ID
resource "viettelidc_ovpc_floating_ip" "bastion" {
  id                   = "5728"
  network_interface_id = viettelidc_ovpc_network_interface.bastion.id
  vpc_id               = data.viettelidc_ovpc_vpc.main.id
}

# 3. Look up an existing Floating IP by Public IP and associate to a Network Interface
data "viettelidc_ovpc_floating_ip" "existing" {
  public_ip = "103.xxx.xxx.xxx"
  vpc_id    = data.viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_floating_ip" "bastion_by_ip" {
  id                   = data.viettelidc_ovpc_floating_ip.existing.id
  network_interface_id = viettelidc_ovpc_network_interface.bastion.id
  vpc_id               = data.viettelidc_ovpc_vpc.main.id
}
