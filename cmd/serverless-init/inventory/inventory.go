// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package inventory adapts serverless-init's cloud-platform knowledge into the
// shared inventoryagent component. All serverless- and platform-specific
// derivation lives here (and in the CloudService structs it delegates to); the
// shared component stays generic, learning about serverless only through its
// neutral Capabilities and the fields injected via its public Set API.
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

// serverlessInitFlavor is the payload flavor emitted by serverless-init. It
// carries the dash intentionally and is injected only into the payload (via
// Set below). serverless-init does not change the process-global flavor
// (flavor.SetFlavor), which stays "agent": the aggregator captures the flavor
// once at construction for the datadog.<flavor>.running / .up heartbeat metric
// and service check, so renaming it process-wide would break agent-host
// identification and existing monitors.
const serverlessInitFlavor = "serverless-init"

// NewCapabilities builds the inventoryagent Capabilities for serverless-init.
// It is supplied to the shared inventoryagent component via fx so the component
// adapts to a hostless, single-process environment (no cross-process
// enrichment, immediate on-start submission, a per-process uuid).
func NewCapabilities() *inventoryagent.Capabilities {
	// A per-process uuid: serverless containers do not share the host machine
	// GUID that uuid.GetUUID() returns.
	return inventoryagent.NewServerlessCapabilities(uuid.New().String())
}

// inventoryEnabled reports whether serverless-init inventory emission is on. It
// is disabled by default while the feature ramps.
//
// TODO(SVLS): gate on util.InventoryEnabled + a serverless-scoped config key
// (DD_SERVERLESS_INIT_INVENTORY_ENABLED) once those keys are added to the
// serverless config schema. Kept as a hardcoded false here so the sketch stays
// at the same "wired but off by default" bar as the A/B spikes.
func inventoryEnabled() bool {
	return false
}

// Inject populates the serverless-specific fields on the shared inventoryagent
// component and enqueues the first payload.
//
// The ordering is deliberate (see INVENTORY_METADATA_PLAN.md, "Scheduling"):
// enable -> Set core+serverless fields -> synchronous submit. The component's
// initData() has already populated the core fields at construction and Set
// overwrites the flavor to serverless-init; the serverless_* fields are layered
// on; then Submit enqueues the payload synchronously so a very short-lived
// container delivers it before exiting rather than racing the runner goroutine.
//
// The inventoryEnabled() guard is defense-in-depth: the component's own Enabled
// gate already makes Set and Submit no-ops, but returning early here keeps the
// intent explicit for the sketch.
func Inject(ia inventoryagent.Component, cs cloudservice.CloudService, modeConf mode.Conf) {
	if !inventoryEnabled() {
		return
	}

	for key, value := range buildFields(cs, modeConf) {
		ia.Set(serverlessFieldPrefix+key, value)
	}
	ia.Set("flavor", serverlessInitFlavor)

	ia.Submit()
}

// buildFields flattens the per-platform inventory data and process-level
// serverless context into the (unprefixed) agent_metadata keys.
func buildFields(cs cloudservice.CloudService, modeConf mode.Conf) map[string]interface{} {
	inv := cs.GetInventoryData()

	return map[string]interface{}{
		"serverless_init_version": serverlessTags.GetExtensionVersion(),

		"resource_id":   inv.ResourceID,
		"resource_name": inv.ResourceName,
		"workload_type": inv.WorkloadType,

		"region":                inv.Region,
		"gcp_project_id":        inv.GCPProjectID,
		"azure_subscription_id": inv.AzureSubscriptionID,

		"deployment_model": deploymentModel(modeConf),
		"runtime":          inv.Runtime,
	}
}

// deploymentModel maps the run mode to the downstream deployment_model value.
func deploymentModel(modeConf mode.Conf) string {
	if modeConf.SidecarMode {
		return "sidecar"
	}
	return "in-container"
}
