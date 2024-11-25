# Configuration-base authentication
provider "viettelidc" {
  domain_id = "3b3e6994-4b04-40ea-bedc-5befd874d73a"
  username  = "iac"
  password  = var.viettelidc_password
  mfa_code  = var.mfa_code
}
