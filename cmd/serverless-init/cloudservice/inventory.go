// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

const (
	// Downstream workload_type values, from the allowlist enforced by the
	// dd-go event-platform-resource-writer service.
	workloadTypeCloudRunService   = "cloud_run_service"
	workloadTypeCloudFunctionGen2 = "cloud_function_gen2"
	workloadTypeCloudRunJob       = "cloud_run_job"
)

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

	// Runtime is the detected workload runtime; likely empty in sidecar mode.
	Runtime string

	// ParentResourceID is the CCRID of the stable parent for revision-capable
	// workloads (e.g. the Cloud Run service behind a revision). Empty when the
	// workload has no distinct parent.
	ParentResourceID string

	// DeploymentID identifies the deployment/revision instance when the platform
	// exposes one; empty otherwise.
	DeploymentID string

	// AzureResourceGroup is the Azure resource group; empty for GCP.
	AzureResourceGroup string
}

// GetInventoryData returns the inventory metadata fields for this cloud
// service. The default implementation returns an empty struct; each platform
// overrides it with real derivation.
//
// TODO(SVLS): derive real per-platform inventory fields.
func (l *LocalService) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for Azure Container
// Apps.
//
// TODO(SVLS): derive azure_container_app workload_type, CCRID, region, and
// subscription id.
func (c *ContainerApp) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for Azure App Service.
//
// TODO(SVLS): derive azure_app_service workload_type, CCRID, region, and
// subscription id.
func (a *AppService) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for AWS MicroVM.
//
// TODO(SVLS): derive the MicroVM workload_type (pending the downstream decoder
// allowlist), CCRID, and region from the MicroVM environment.
func (m *MicroVM) GetInventoryData() InventoryData { return InventoryData{} }
