// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package vpc implements ViettelIDC IaC Phase 4 autoscaling resources
// and data sources for the Plugin Framework provider:
//   - viettelidc_launch_template (resource + 2 data sources)
//   - viettelidc_autoscale_group (resource + 1 data source)
//
// All resources call API via API Gateway using /terraform/v1/vpc/... paths.
// The API Gateway rewrites these to /csa/api/v1/vpc/... and
// renames snake_case body fields to camelCase.
package vpc

// Routed CSA endpoint paths for Phase 4 VPC autoscaling resources.
// All endpoints accept HTTP POST. These paths use the /terraform/v1/vpc/
// prefix; the API Gateway rewrites them
// to /csa/api/v1/vpc/... before forwarding to API.
const (
	pathLaunchTemplateCreate  = "/terraform/v1/vpc/launch-template/create"
	pathLaunchTemplateDetail  = "/terraform/v1/vpc/launch-template/detail"
	pathLaunchTemplateDelete  = "/terraform/v1/vpc/launch-template/delete"
	pathLaunchTemplateList    = "/terraform/v1/vpc/launch-template/list"
	pathLaunchTemplateListAll = "/terraform/v1/vpc/launch-template/list-all"

	pathAutoscaleGroupCreate = "/terraform/v1/vpc/autoscale-group/create"
	pathAutoscaleGroupList   = "/terraform/v1/vpc/autoscale-group/list"
	pathAutoscaleGroupDelete = "/terraform/v1/vpc/autoscale-group/delete"
	// NOTE: No autoscale-group-detail path — the API has no detail endpoint for ASG.
	// Read() uses list+filter instead (see Decision 8 in architecture.md).
)

// listWarningThreshold triggers a Diagnostics warning on list-style data
// sources when the result count meets/exceeds this value (likely truncated).
const listWarningThreshold = 1000
