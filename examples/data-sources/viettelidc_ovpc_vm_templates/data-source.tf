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

# Not every template accepts an SSH key. Appliance images (GitLab, Jenkins,
# Acunetix, ...) come back with ssh_key_enabled = false — key_pair_name is
# ignored on those and you have to log in with admin_pass. Pick one that
# accepts a key:
locals {
  key_templates = [
    for t in data.viettelidc_ovpc_vm_templates.ubuntu.templates : t if t.ssh_key_enabled
  ]
}
