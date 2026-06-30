// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package dbs implements ViettelIDC IaC Phase 5 DBS (Database Service) resources
// for the Plugin Framework provider:
//   - viettelidc_vdbs_database_instance (resource)
//   - viettelidc_vdbs_subnet_group (resource)
//   - viettelidc_vdbs_security_group (resource)
//   - viettelidc_vdbs_parameter_group (resource)
//
// Write operations (Create/Update/Delete) for DB Instance route through DBS:
//
//	POST /dbs/api/v1/dbs/instance/...
//
// Read operations and all Subnet/SG/Parameter operations route through CSA:
//
//	POST /csa/api/v1/dbaas/...
//
// Kong handles routing and renames snake_case fields to camelCase (Decision 9).
package dbs

// DB Instance paths — dual-backend (Decision 9).
// Writes go to DBS service; reads go to CSA service.
const (
	pathDBInstanceCreate = "/dbs/api/v1/dbs/instance/create"
	pathDBInstanceUpdate = "/dbs/api/v1/dbs/instance/update"
	pathDBInstanceDelete = "/dbs/api/v1/dbs/instance/delete"
	pathDBInstanceDetail = "/csa/api/v1/dbaas/instance/detail"
	pathDBInstanceList   = "/csa/api/v1/dbaas/instance/list"

	// Subnet Group paths (CSA).
	// Note: Subnet groups and subnets share the same list endpoint on the API
	pathSubnetGroupCreate = "/csa/api/v1/dbaas/subnet-group/create"
	pathSubnetGroupList   = "/csa/api/v1/dbaas/subnet/list"
	pathSubnetGroupUpdate = "/csa/api/v1/dbaas/subnet-group/update"
	pathSubnetGroupDelete = "/csa/api/v1/dbaas/subnet-group/delete"

	// DBS Subnet paths (CSA) - detail lookup for existing subnets.
	pathDBSSubnetDetail = "/csa/api/v1/dbaas/subnet/detail"

	// DBS Network Interface paths (CSA) - detail lookup for existing ENIs.
	pathDBSNICDetail = "/csa/api/v1/dbaas/nic/detail"
	pathDBSNICList   = "/csa/api/v1/dbaas/nic/list"

	// DBS Subnet list (CSA).
	pathDBSSubnetList = "/csa/api/v1/dbaas/subnet/list"

	// DB Instance lifecycle action paths (CSA).
	pathDBInstanceStart   = "/csa/api/v1/dbaas/instance/start"
	pathDBInstanceStop    = "/csa/api/v1/dbaas/instance/stop"
	pathDBInstanceReboot  = "/csa/api/v1/dbaas/instance/reboot"
	pathDBInstancePromote = "/csa/api/v1/dbaas/instance/promote"

	// DBS Security Group paths (CSA).
	pathDBSGCreate          = "/csa/api/v1/dbaas/security-group/create"
	pathDBSGDetail          = "/csa/api/v1/dbaas/security-group/detail"
	pathDBSGList            = "/csa/api/v1/dbaas/security-group/list"
	pathDBSGDelete          = "/csa/api/v1/dbaas/security-group/delete"
	pathDBSGRuleCreate      = "/csa/api/v1/dbaas/security-group/rule/create"
	pathDBSGRuleDelete      = "/csa/api/v1/dbaas/security-group/rule/delete"
	pathDBSGRuleUpdate      = "/csa/api/v1/dbaas/security-group/rule/update"
	pathDBSGRuleInboundList = "/csa/api/v1/dbaas/security-group/rule/inbound/list"

	// Parameter Group paths (DBS RESTful).
	pathParamGroupCreate     = "/dbs/api/v1/configuration/parameter-group"
	pathParamGroupUpdate     = "/dbs/api/v1/configuration/parameter-group"
	pathParamGroupRename     = "/dbs/api/v1/configuration/parameter-group/rename"
	pathParamGroupAttach     = "/dbs/api/v1/configuration/parameter-group/attach"
	pathParamGroupDetail     = "/dbs/api/v1/configuration/parameter-group/%s"
	pathParamGroupParameters = "/dbs/api/v1/configuration/parameter-group/%s/parameters"
	pathParamGroupDelete     = "/dbs/api/v1/configuration/parameter-group/%s"
	pathParamGroupList       = "/dbs/api/v1/configuration/parameter-group"
	pathVpcList              = "/dbs/api/v1/extend/vpc/customer/list"
	pathDatastoreVersionList = "/dbs/api/v1/extend/datastore/version/list"

	// Backup paths
	pathBackupList            = "/dbs/api/v1/backup/customer/list"
	pathBackupSchedulerList   = "/dbs/api/v1/scheduler/backup/customer/list"
	pathBackupSchedulerCreate = "/dbs/api/v1/scheduler/backup/create"
	pathBackupSchedulerEdit   = "/dbs/api/v1/scheduler/backup/edit"
	pathBackupSchedulerDelete = "/dbs/api/v1/scheduler/backup/delete"

	// DB User & Schema paths
	pathDBUserList   = "/dbs/api/v1/user/list-paging"
	pathDBUserCreate = "/dbs/api/v1/user/create"
	pathDBUserUpdate = "/dbs/api/v1/user/update"
	pathDBUserDelete = "/dbs/api/v1/user/delete"
	pathDBUserGrant  = "/dbs/api/v1/user/grant"
	pathDBUserRevoke = "/dbs/api/v1/user/revoke"
	pathDBSchemaList = "/dbs/api/v1/schema/list"
)
