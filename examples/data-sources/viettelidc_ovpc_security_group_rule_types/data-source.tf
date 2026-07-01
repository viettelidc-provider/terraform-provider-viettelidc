data "viettelidc_ovpc_security_group_rule_types" "all" {}

output "rule_type_names" {
  value = [for t in data.viettelidc_ovpc_security_group_rule_types.all.rule_types : t.name]
}
