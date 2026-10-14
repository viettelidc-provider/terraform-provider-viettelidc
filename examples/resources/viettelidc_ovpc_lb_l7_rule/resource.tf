data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# 1. Match URL path prefix (e.g. /api/)
resource "viettelidc_ovpc_lb_l7_rule" "api_path" {
  vpc_id       = data.viettelidc_ovpc_vpc.main.id
  policy_id    = viettelidc_ovpc_lb_l7_policy.route_to_api_pool.id
  type         = "PATH"
  compare_type = "STARTS_WITH"
  value        = "/api/"
}

# 2. Match Host header (FQDN)
resource "viettelidc_ovpc_lb_l7_rule" "host_header" {
  vpc_id       = data.viettelidc_ovpc_vpc.main.id
  policy_id    = viettelidc_ovpc_lb_l7_policy.route_to_api_pool.id
  type         = "HOST_NAME"
  compare_type = "EQUAL_TO"
  value        = "api.viettelidc.com.vn"
}

# 3. Match custom HTTP header
resource "viettelidc_ovpc_lb_l7_rule" "custom_header" {
  vpc_id       = data.viettelidc_ovpc_vpc.main.id
  policy_id    = viettelidc_ovpc_lb_l7_policy.reject_bots.id
  type         = "HEADER"
  compare_type = "CONTAINS"
  key          = "User-Agent"
  value        = "Scrapy"
}

# 4. Match Cookie value
resource "viettelidc_ovpc_lb_l7_rule" "beta_cookie" {
  vpc_id       = data.viettelidc_ovpc_vpc.main.id
  policy_id    = viettelidc_ovpc_lb_l7_policy.route_to_api_pool.id
  type         = "COOKIE"
  compare_type = "EQUAL_TO"
  key          = "app_channel"
  value        = "canary"
}

# 5. Match static file extension (note: must NOT start with dot)
resource "viettelidc_ovpc_lb_l7_rule" "static_images" {
  vpc_id       = data.viettelidc_ovpc_vpc.main.id
  policy_id    = viettelidc_ovpc_lb_l7_policy.route_to_api_pool.id
  type         = "FILE_TYPE"
  compare_type = "EQUAL_TO"
  value        = "jpg" # Must NOT start with dot
}
