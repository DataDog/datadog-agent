// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

// InventoryData holds the per-platform serverless fields that feed the
// serverless-init inventory metadata payload. Each CloudService implementation
// derives these from its own environment so the payload builder stays thin and
// the derivation lives next to the existing tag logic.
//
// The zero value of every field is a valid "not applicable / unknown" value.
// Nullable downstream columns map from empty strings here.
type InventoryData struct {
	// WorkloadType is the canonical workload type, e.g. cloud_run_service,
	// cloud_run_job, cloud_function_gen2, azure_container_app,
	// azure_app_service.
	WorkloadType string

	// ResourceID is the Canonical Cloud Resource ID (CCRID); the first
	// component of the downstream composite key.
	ResourceID string

	// ResourceName is the platform display name (app / job / revision). It is
	// never substituted with dd_service.
	ResourceName string

	// Region is the GCP or Azure region.
	Region string

	// GCPProjectID is set for GCP platforms; empty for Azure.
	GCPProjectID string

	// AzureSubscriptionID is set for Azure platforms; empty for GCP.
	AzureSubscriptionID string

	// GCPDeploymentType is one of Function|Source|Container|Repo; empty for
	// non-GCP.
	GCPDeploymentType string

	// AzureHostingPlan is one of Consumption|Flex; empty for non-Azure.
	AzureHostingPlan string

	// AzureDeploymentType is one of Code|Container; empty for non-Azure.
	AzureDeploymentType string

	// WorkloadRuntime is the detected workload runtime; likely empty in
	// sidecar mode.
	WorkloadRuntime string
}

// GetInventoryData returns the inventory metadata fields for this cloud
// service. The default implementation returns an empty struct; each platform
// overrides it with real derivation.
//
// TODO(SVLS): derive real per-platform inventory fields.
func (l *LocalService) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for Cloud Run
// (service, function).
//
// TODO(SVLS): derive workload_type (service vs cloud_function_gen2), CCRID,
// region, project id, and GCP deployment type from the Cloud Run environment.
func (c *CloudRun) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for Cloud Run Jobs.
//
// TODO(SVLS): derive cloud_run_job workload_type, CCRID, region, and project id.
func (c *CloudRunJobs) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for Azure Container
// Apps.
//
// TODO(SVLS): derive azure_container_app workload_type, CCRID, region,
// subscription id, hosting plan, and deployment type.
func (c *ContainerApp) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for Azure App Service.
//
// TODO(SVLS): derive azure_app_service workload_type, CCRID, region,
// subscription id, hosting plan, and deployment type.
func (a *AppService) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for AWS MicroVM.
//
// TODO(SVLS): derive the MicroVM workload_type (pending the downstream decoder
// allowlist), CCRID, and region from the MicroVM environment.
func (m *MicroVM) GetInventoryData() InventoryData { return InventoryData{} }
