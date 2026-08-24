// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverlessinventory exposes the interface for the component that
// generates the serverless-init inventory metadata payload.
package serverlessinventory

// team: serverless-azure-gcp

// Component is the component type.
type Component interface {
	// Refresh triggers a new payload to be sent while still respecting the
	// minimal interval between two updates.
	Refresh()
}

// Fields is the typed contract for the serverless-specific inventory fields.
// It is the single source of truth for the field set: each JSON tag is the
// downstream key name (without the serverless_ prefix, which the component
// applies uniformly). The zero value of a field maps to a nullable/unknown
// downstream column.
//
// TODO(SVLS): additional fields are added here as they are wired up: DD_*
// passthrough (dd_env, dd_site, dd_version, dd_service), core lineage
// duplicates (agent_version_base, agent_commit), and wrapped_command.
type Fields struct {
	// InitVersion is the serverless-init extension version. It is REQUIRED and
	// gates emission. The serverless_ prefix applied by the component makes the
	// final key serverless_serverless_init_version (intentional double prefix).
	InitVersion string `json:"serverless_init_version"`

	// ResourceID is the Canonical Cloud Resource ID (CCRID); REQUIRED, first
	// component of the downstream composite key.
	ResourceID string `json:"resource_id"`
	// ResourceName is the platform display name (app/job/revision); REQUIRED.
	ResourceName string `json:"resource_name"`
	// WorkloadType is the canonical workload type; REQUIRED.
	WorkloadType string `json:"workload_type"`

	// Region is the GCP or Azure region.
	Region string `json:"region"`
	// GCPProjectID is set for GCP platforms; empty for Azure.
	GCPProjectID string `json:"gcp_project_id"`
	// AzureSubscriptionID is set for Azure platforms; empty for GCP.
	AzureSubscriptionID string `json:"azure_subscription_id"`

	// DeploymentModel is sidecar or in-container.
	DeploymentModel string `json:"deployment_model"`
	// GCPDeploymentType is one of Function|Source|Container|Repo; empty for non-GCP.
	GCPDeploymentType string `json:"gcp_deployment_type"`
	// AzureHostingPlan is one of Consumption|Flex; empty for non-Azure.
	AzureHostingPlan string `json:"azure_hosting_plan"`
	// AzureDeploymentType is one of Code|Container; empty for non-Azure.
	AzureDeploymentType string `json:"azure_deployment_type"`
	// WorkloadRuntime is the detected workload runtime; likely empty in sidecar mode.
	WorkloadRuntime string `json:"workload_runtime"`
}

// FieldProvider supplies the serverless-specific fields that are layered into
// the payload's agent_metadata map. It is implemented outside this component
// (in cmd/serverless-init) so all serverless- and cloud-platform-specific
// derivation stays there; the component only knows how to assemble and submit
// a payload.
type FieldProvider interface {
	// GetInventoryFields returns the serverless-specific fields for this
	// process.
	GetInventoryFields() Fields
}
