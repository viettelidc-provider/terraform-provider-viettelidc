// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package vpc implements ViettelIDC IaC Phase 4 autoscaling resources
// and data sources for the Plugin Framework provider:
//   - viettelidc_launch_template (resource + 2 data sources)
//   - viettelidc_autoscale_group (resource + 1 data source)
package vpc

// Routed CSA endpoint paths for Phase 4 VPC autoscaling resources.
// All endpoints accept HTTP POST. These paths use the /csa/api/v1/vpc/
// prefix matching the pattern used by networking resources.
const (
	pathLaunchTemplateCreate  = "/terraform/v1/vpc/launch-template/create"
	pathLaunchTemplateDetail  = "/terraform/v1/vpc/launch-template/detail"
	pathLaunchTemplateDelete  = "/terraform/v1/vpc/launch-template/delete"
	pathLaunchTemplateList    = "/terraform/v1/vpc/launch-template/list"
	pathLaunchTemplateListAll = "/terraform/v1/vpc/launch-template/list-all"

	pathAutoscaleGroupCreate = "/terraform/v1/vpc/autoscale-group/create"
	pathAutoscaleGroupList   = "/terraform/v1/vpc/autoscale-group/list"
	pathAutoscaleGroupDelete = "/terraform/v1/vpc/autoscale-group/delete"
)

// listWarningThreshold triggers a Diagnostics warning on list-style data
// sources when the result count meets/exceeds this value (likely truncated).
const listWarningThreshold = 1000
