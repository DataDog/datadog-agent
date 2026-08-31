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
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	serverlessTags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// reportReasonStartup marks the primary synchronous on-start submission. It is
// telemetry-only downstream (a bounded metric dimension) and not persisted.
const reportReasonStartup = "startup"

// serverlessFieldPrefix is applied uniformly to every serverless-specific key
// in agent_metadata.
const serverlessFieldPrefix = "serverless_"

// serverlessInitFlavor is the payload flavor emitted by serverless-init. It is
// injected only into the payload (via Set below), not the process-global flavor
// (flavor.SetFlavor): the aggregator captures that once for the
// datadog.<flavor>.running/.up heartbeat metric and service check, so renaming
// it process-wide would break agent-host identification and existing monitors.
const serverlessInitFlavor = "serverless-init"

// NewCapabilities builds the inventoryagent Capabilities for serverless-init,
// with a per-process uuid since serverless containers do not share a host GUID.
func NewCapabilities() *inventoryagent.Capabilities {
	return inventoryagent.NewServerlessCapabilities(uuid.New().String())
}

// Inject layers the serverless-specific fields and the serverless-init flavor
// onto the shared inventoryagent component via its public Set API. The
// component's initData() has already populated the core fields at construction.
//
// Inject, Submit, and SetDeploymentID are all no-ops while the
// serverless.inventory_enabled ramp gate is off, so a gated-off run emits no
// serverless payload at all rather than one carrying only core fields.
func Inject(ia inventoryagent.Component, cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	for key, value := range buildFields(cs, modeConf, conf, tags) {
		ia.Set(serverlessFieldPrefix+key, value)
	}
	ia.Set("flavor", serverlessInitFlavor)
}

// Submit enqueues an inventory payload now, synchronously, so a short-lived
// container delivers it before exiting rather than racing the runner goroutine.
func Submit(ia inventoryagent.Component, conf configmodel.Reader) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	ia.Submit()
}

// SetDeploymentID sets the deployment_id serverless field, for platforms that
// only learn their deployment/instance identifier after the initial Inject
// (e.g. delivered by a lifecycle hook rather than the environment).
func SetDeploymentID(ia inventoryagent.Component, conf configmodel.Reader, id string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	ia.Set(serverlessFieldPrefix+"deployment_id", id)
}

// buildFields flattens the per-platform inventory data and process-level
// serverless context into the (unprefixed) agent_metadata keys.
//
// DD_* passthrough: env and site are real config keys, but version and service
// are not (DD_VERSION / DD_SERVICE are read by the agent outside the Config
// struct), so they come from the already-computed tag map rather than
// conf.GetString, which would return empty and log an unknown-key warning.
func buildFields(cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) map[string]interface{} {
	inv := cs.GetInventoryData()

	fields := map[string]interface{}{
		"serverless_init_version": serverlessTags.GetExtensionVersion(),
		"agent_version_base":      version.AgentVersion,
		"agent_commit":            version.Commit,
		"report_reason":           reportReasonStartup,

		"resource_id":        inv.ResourceID,
		"resource_name":      inv.ResourceName,
		"workload_type":      inv.WorkloadType,
		"parent_resource_id": inv.ParentResourceID,
		"deployment_id":      inv.DeploymentID,

		"region":                inv.Region,
		"gcp_project_id":        inv.GCPProjectID,
		"azure_subscription_id": inv.AzureSubscriptionID,
		"azure_resource_group":  inv.AzureResourceGroup,

		"deployment_model": deploymentModel(modeConf),
		"runtime":          inv.Runtime,

		"dd_env":     conf.GetString("env"),
		"dd_site":    conf.GetString("site"),
		"dd_version": tags["version"],
		"dd_service": tags["service"],
	}

	// wrapped_command is the customer workload command wrapped by serverless-init
	// in init mode (os.Args[1:]); it is absent in sidecar mode, where
	// serverless-init wraps nothing.
	if !modeConf.SidecarMode && len(os.Args) > 1 {
		fields["wrapped_command"] = strings.Join(os.Args[1:], " ")
	}

	return fields
}

// deploymentModel maps the run mode to the downstream deployment_model value.
// Like workload_type, these strings are an allowlist enforced by the dd-go
// event-platform-resource-writer decoder; a value outside it is rejected there.
func deploymentModel(modeConf mode.Conf) string {
	if modeConf.SidecarMode {
		return "sidecar"
	}
	return "in-container"
}
