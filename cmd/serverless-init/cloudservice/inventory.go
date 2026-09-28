// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

const (
	workloadTypeCloudRunService   = "cloud_run_service"
	workloadTypeCloudRunFunction  = "cloud_run_function"
	workloadTypeCloudRunJob       = "cloud_run_job"
	workloadTypeAzureContainerApp = "azure_container_app"
	workloadTypeAzureAppService   = "azure_app_service"
	workloadTypeAzureFunction     = "azure_function"
)

// InventoryData holds the per-platform serverless fields that feed the
// serverless-init inventory metadata payload. Each CloudService implementation
// derives these from its own environment so the payload builder stays thin and
// the derivation lives next to the existing tag logic.
//
// Empty strings represent unavailable values. The serverless-init payload builder
// converts them to nil, allowing optional fields to be omitted or serialized as
// JSON null. Missing required identity remains subject to CanCollectInventory.
type InventoryData struct {
	WorkloadType string

	// ResourceID is the Canonical Cloud Resource ID (CCRID), the first component
	// of the downstream composite key.
	ResourceID string

	// ResourceName is the platform display name (app / service / job); it is
	// never substituted with dd_service.
	ResourceName string

	Region              string
	GCPProjectID        string
	AWSAccountID        string
	AzureSubscriptionID string
	AzureResourceGroup  string

	// RuntimeCandidates are raw application-runtime values in priority order.
	// The inventory resolver normalizes them and selects the first usable value.
	RuntimeCandidates []string

	// ParentResourceID is the CCRID of the semantic parent (e.g. the Cloud Run
	// service behind a revision), not necessarily a string prefix of ResourceID.
	// Empty when the workload has no distinct parent.
	ParentResourceID string
}

func (l *LocalService) CanCollectInventory() bool       { return true }
func (l *LocalService) GetInventoryData() InventoryData { return InventoryData{} }

func (m *MicroVM) CanCollectInventory() bool       { return false }
func (m *MicroVM) GetInventoryData() InventoryData { return InventoryData{} }
