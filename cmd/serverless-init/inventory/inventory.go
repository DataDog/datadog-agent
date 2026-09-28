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
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// reportReasonStartup marks the primary synchronous on-start submission. It is
// telemetry-only downstream (a bounded metric dimension) and not persisted.
const reportReasonStartup = "startup"
const reportReasonPeriodic = "periodic"

// serverlessInitFlavor is the payload flavor emitted by serverless-init. It is
// injected only into the payload (via Set below), not the process-global flavor
// (flavor.SetFlavor): the aggregator captures that once for the
// datadog.<flavor>.running/.up heartbeat metric and service check, so renaming
// it process-wide would break agent-host identification and existing monitors.
const serverlessInitFlavor = "serverless-init"

// NewCapabilities builds the inventoryagent Capabilities for serverless-init,
// reporting one uuid for the process lifetime since serverless containers do not
// share a host GUID.
func NewCapabilities() *inventoryagent.Capabilities {
	id := uuid.New().String()
	return inventoryagent.NewServerlessCapabilities(func() string { return id })
}

// Inject layers the serverless-specific fields and the serverless-init flavor
// onto the shared inventoryagent component via its public Set API. The
// component's initData() has already populated the core fields at construction.
//
// Inject and Submit are no-ops while the
// serverless.inventory_enabled ramp gate is off, so a gated-off run emits no
// serverless payload at all rather than one carrying only core fields.
func Inject(ia inventoryagent.Component, cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	for key, value := range buildFields(cs, modeConf, conf, tags) {
		ia.Set(key, value)
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
	ia.Set("report_reason", reportReasonPeriodic)
}

// buildFields flattens the per-platform inventory data and process-level
// serverless context into the (unprefixed) agent_metadata keys.
//
// The three Unified Service Tagging fields come from the already-computed tag
// map, so inventory reports the same env/service/version this container tags its
// metrics, logs, and traces with: the map is lowercased and lets DD_TAGS /
// DD_EXTRA_TAGS override the DD_ENV / DD_SERVICE / DD_VERSION values. Reading
// them from the config instead would diverge on both counts, and service and
// version are not even config keys (DD_SERVICE / DD_VERSION are read by the
// agent outside the Config struct), so conf.GetString would return empty and log
// an unknown-key warning. site is not a tag, so it comes from the config.
func buildFields(cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) map[string]interface{} {
	inv := cs.GetInventoryData()
	var wrappedCommand []string
	if !modeConf.SidecarMode && len(os.Args) > 1 {
		wrappedCommand = os.Args[1:]
	}

	fields := map[string]interface{}{
		"serverless_init_version": serverlessTags.GetExtensionVersion(),
		"agent_version_base":      version.AgentVersion,
		"agent_commit":            version.Commit,
		"report_reason":           reportReasonStartup,

		"resource_id":        inv.ResourceID,
		"resource_name":      inv.ResourceName,
		"workload_type":      inv.WorkloadType,
		"parent_resource_id": inv.ParentResourceID,

		"region":                inv.Region,
		"gcp_project_id":        inv.GCPProjectID,
		"aws_account_id":        inv.AWSAccountID,
		"azure_subscription_id": inv.AzureSubscriptionID,
		"azure_resource_group":  inv.AzureResourceGroup,

		"deployment_model": deploymentModel(modeConf),
		"runtime":          resolveRuntime(inv.RuntimeCandidates, wrappedCommand),

		"dd_env":     tags["env"],
		"dd_site":    conf.GetString("site"),
		"dd_version": tags["version"],
		"dd_service": tags["service"],
	}

	// wrapped_command is the customer workload command wrapped by serverless-init
	// in init mode (os.Args[1:]); it is absent in sidecar mode, where
	// serverless-init wraps nothing. Scrubbed before storage: command-line
	// arguments can contain credentials (e.g. --password=secret, --token=…).
	if len(wrappedCommand) > 0 {
		fields["wrapped_command"] = scrubber.ScrubLine(strings.Join(wrappedCommand, " "))
	}

	for key, value := range fields {
		if text, ok := value.(string); ok && text == "" {
			// Passing nil also clears a previously populated value in the component's cache.
			fields[key] = nil
		}
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
