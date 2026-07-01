resource "viettelidc_ovpc_key_pair" "deploy" {
  key_name = "deploy-key"
  vpc_id   = viettelidc_ovpc_vpc.main.id
}

output "private_key_url" {
  value     = viettelidc_ovpc_key_pair.deploy.download_url
  sensitive = true
}
