# List all templates whose name contains "Ubuntu 22.04"
data "viettelidc_ovpc_vm_templates" "ubuntu" {
  name_filter = "Ubuntu 22.04"
  host_id     = 6
}

# Use the first matching template when creating an instance
resource "viettelidc_ovpc_instance" "vm1" {
  template_id = data.viettelidc_ovpc_vm_templates.ubuntu.templates[0].id
  # ...
}
