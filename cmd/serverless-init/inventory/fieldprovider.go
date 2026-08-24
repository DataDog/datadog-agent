// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package inventory adapts serverless-init's cloud-platform knowledge into the
// serverlessinventory.FieldProvider interface consumed by the
// comp/metadata/serverlessinventory component. All serverless- and
// platform-specific derivation lives here (and in the CloudService structs it
// delegates to); the component stays generic.
package inventory

import (
	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessinventory "github.com/DataDog/datadog-agent/comp/metadata/serverlessinventory/def"
	serverlessTags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

// fieldProvider implements serverlessinventory.FieldProvider by combining the
// per-platform CloudService inventory data with process-level serverless
// context (run mode, extension version).
type fieldProvider struct {
	cloudService cloudservice.CloudService
	modeConf     mode.Conf
}

// NewFieldProvider builds a serverlessinventory.FieldProvider for the detected
// cloud service and run mode. It is supplied to the serverlessinventory
// component via fx.
func NewFieldProvider(cloudService cloudservice.CloudService, modeConf mode.Conf) serverlessinventory.FieldProvider {
	return &fieldProvider{cloudService: cloudService, modeConf: modeConf}
}

// GetInventoryFields returns the typed serverless-specific fields; the
// component applies the serverless_ prefix.
func (p *fieldProvider) GetInventoryFields() serverlessinventory.Fields {
	inv := p.cloudService.GetInventoryData()

	return serverlessinventory.Fields{
		InitVersion:  serverlessTags.GetExtensionVersion(),
		ResourceID:   inv.ResourceID,
		ResourceName: inv.ResourceName,
		WorkloadType: inv.WorkloadType,

		Region:              inv.Region,
		GCPProjectID:        inv.GCPProjectID,
		AzureSubscriptionID: inv.AzureSubscriptionID,

		DeploymentModel:     p.deploymentModel(),
		GCPDeploymentType:   inv.GCPDeploymentType,
		AzureHostingPlan:    inv.AzureHostingPlan,
		AzureDeploymentType: inv.AzureDeploymentType,
		WorkloadRuntime:     inv.WorkloadRuntime,
	}
}

// deploymentModel maps the run mode to the downstream deployment_model value.
func (p *fieldProvider) deploymentModel() string {
	if p.modeConf.SidecarMode {
		return "sidecar"
	}
	return "in-container"
}
