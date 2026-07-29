data "viettelidc_ovpc_certificate" "tls" {
  name   = "my-tls-cert"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

# load_balancers lists what is using the certificate. A cert in use cannot be
# deleted, so check this before removing one.
output "cert_in_use_by" {
  value = data.viettelidc_ovpc_certificate.tls.load_balancers
}
