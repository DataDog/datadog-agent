// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package inventory adapts serverless-init's cloud-platform knowledge into the
// inventoryagent component's Params. All serverless- and platform-specific
// derivation lives here (and in the CloudService structs it delegates to); the
// shared inventoryagent component stays generic and only learns about
// serverless through the typed Params contract.
package inventory

import (
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	serverlessTags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

// serverlessFieldPrefix is applied uniformly to every serverless-specific key
// in agent_metadata.
const serverlessFieldPrefix = "serverless_"

// BuildParams assembles the inventoryagent.Params for the detected cloud
// service and run mode. It is supplied to the inventoryagent component via fx
// so serverless-init can reuse the shared component. The serverless payload
// flavor, per-process uuid, and no-cross-process-fetch defaults are encoded by
// inventoryagent.NewServerlessParams.
func BuildParams(cloudService cloudservice.CloudService, modeConf mode.Conf) *inventoryagent.Params {
	return inventoryagent.NewServerlessParams(
		inventoryEnabled(),
		// A per-process uuid: serverless containers do not share the host
		// machine GUID that uuid.GetUUID() returns.
		uuid.New().String(),
		buildExtraFields(cloudService, modeConf),
	)
}

// inventoryEnabled reports whether serverless-init inventory emission is on. It
// is disabled by default while the feature ramps.
//
// TODO(SVLS): gate on util.InventoryEnabled + a serverless-scoped config key
// (DD_SERVERLESS_INIT_INVENTORY_ENABLED) once those keys are added to the
// serverless config schema. Kept as a hardcoded false here so the spike stays
// at the same "wired but off by default" bar as the serverlessinventory spike.
func inventoryEnabled() bool {
	return false
}

// buildExtraFields flattens the per-platform inventory data and process-level
// serverless context into the prefixed agent_metadata keys.
func buildExtraFields(cloudService cloudservice.CloudService, modeConf mode.Conf) map[string]interface{} {
	inv := cloudService.GetInventoryData()

	fields := map[string]interface{}{
		"serverless_init_version": serverlessTags.GetExtensionVersion(),

		"resource_id":   inv.ResourceID,
		"resource_name": inv.ResourceName,
		"workload_type": inv.WorkloadType,

		"region":                inv.Region,
		"gcp_project_id":        inv.GCPProjectID,
		"azure_subscription_id": inv.AzureSubscriptionID,

		"deployment_model":      deploymentModel(modeConf),
		"gcp_deployment_type":   inv.GCPDeploymentType,
		"azure_hosting_plan":    inv.AzureHostingPlan,
		"azure_deployment_type": inv.AzureDeploymentType,
		"workload_runtime":      inv.WorkloadRuntime,
	}

	prefixed := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		prefixed[serverlessFieldPrefix+key] = value
	}
	return prefixed
}

// deploymentModel maps the run mode to the downstream deployment_model value.
func deploymentModel(modeConf mode.Conf) string {
	if modeConf.SidecarMode {
		return "sidecar"
	}
	return "in-container"
}
