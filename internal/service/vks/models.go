// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import "github.com/hashicorp/terraform-plugin-framework/types"

// ClusterResourceModel maps the Terraform schema for a VKS Cluster.
type ClusterResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Status   types.String `tfsdk:"status"`
	Version  types.String `tfsdk:"version"`
	Endpoint types.String `tfsdk:"endpoint"`
	VpcID    types.String `tfsdk:"vpc_id"`
}

// NodeGroupResourceModel maps the Terraform schema for a VKS Node Group.
type NodeGroupResourceModel struct {
	ID           types.String `tfsdk:"id"`
	ClusterID    types.String `tfsdk:"cluster_id"`
	Name         types.String `tfsdk:"name"`
	InstanceType types.String `tfsdk:"instance_type"`
	MinSize      types.Int64  `tfsdk:"min_size"`
	MaxSize      types.Int64  `tfsdk:"max_size"`
	DesiredSize  types.Int64  `tfsdk:"desired_size"`
	Status       types.String `tfsdk:"status"`
}
