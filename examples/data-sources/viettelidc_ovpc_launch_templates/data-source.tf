data "viettelidc_ovpc_launch_templates" "all" {
  vpc_id = viettelidc_ovpc_vpc.main.id
}

output "template_names" {
  value = [for t in data.viettelidc_ovpc_launch_templates.all.launch_templates : t.name]
}
