// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"

	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
)

const (
	// Downstream workload_type values, from the allowlist enforced by the
	// dd-go event-platform-resource-writer service.
	workloadTypeCloudRunService   = "cloud_run_service"
	workloadTypeCloudRunFunction  = "cloud_run_function"
	workloadTypeCloudRunJob       = "cloud_run_job"
	workloadTypeAzureContainerApp = "azure_container_app"
	workloadTypeAzureAppService   = "azure_app_service"
	workloadTypeAzureFunction     = "azure_function"
	workloadTypeAWSMicroVM        = "aws_microvm"
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

	// ResourceName is the platform display name (app / job / revision); it is
	// never substituted with dd_service.
	ResourceName string

	Region              string
	GCPProjectID        string
	AWSAccountID        string
	AzureSubscriptionID string
	AzureResourceGroup  string
	Runtime             string

	// ParentResourceID is the CCRID of the immediate stable parent one level
	// above ResourceID (e.g. the Cloud Run service behind a revision). Empty when
	// the workload has no distinct parent.
	ParentResourceID string
}

func (l *LocalService) CanCollectInventory() bool { return true }

// MicroVM identity is supplied by lifecycle hooks after component construction.
func (m *MicroVM) CanCollectInventory() bool { return true }

// GetInventoryData returns the inventory metadata fields for this cloud
// service. The default implementation returns an empty struct; each platform
// overrides it with real derivation.
//
// TODO(SVLS): derive real per-platform inventory fields.
func (l *LocalService) GetInventoryData() InventoryData { return InventoryData{} }

// GetInventoryData returns the inventory metadata fields for AWS MicroVM,
// derived from the image ARN env var. The image is the stable parent every
// instance runs from, so it is the ParentResourceID.
//
// The per-instance MicroVM id is not known at derivation time (the platform
// only delivers it in the /run lifecycle hook body), so ResourceID starts as
// the image ARN and narrows to the instance id at submission time. That keeps
// resource_id populated for a payload built before the first /run.
func (m *MicroVM) GetInventoryData() InventoryData {
	arn := os.Getenv(serverlessenv.MicroVMImageARNEnvVar)
	if arn == "" {
		return InventoryData{WorkloadType: workloadTypeAWSMicroVM}
	}
	region, accountID, imageName := parseMicroVMARN(arn)
	return InventoryData{
		WorkloadType:     workloadTypeAWSMicroVM,
		ResourceID:       arn,
		ParentResourceID: arn,
		ResourceName:     imageName,
		Region:           region,
		AWSAccountID:     accountID,
	}
}
