// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package inventory builds the serverless-init inventory payload, flattening
// CloudService platform facts and process-level context into the
// inventoryagent component's metadata fields.
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

const reportReasonStartup = "startup"
const reportReasonPeriodic = "periodic"

// serverlessInitFlavor is the flavor reported in the inventory payload only;
// it is not the process-global flavor (flavor.SetFlavor).
const serverlessInitFlavor = "serverless-init"

// NewCapabilities builds initially-not-ready inventoryagent Capabilities for
// serverless-init, reporting one uuid for the process lifetime since serverless
// containers do not share a host GUID.
func NewCapabilities() *inventoryagent.Capabilities {
	id := uuid.New().String()
	return inventoryagent.NewServerlessCapabilities(func() string { return id })
}

// UpdateAndSubmit updates all serverless metadata and submits the startup
// payload.
func UpdateAndSubmit(ia inventoryagent.Component, cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	ia.SetReady(false)
	Inject(ia, cs, modeConf, conf, tags)
	ia.SetReady(true)
	Submit(ia, conf)
}

// Inject sets the serverless-specific fields on the inventoryagent component.
// Readiness must be closed while injecting.
func Inject(ia inventoryagent.Component, cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	for key, value := range buildFields(cs, modeConf, conf, tags) {
		ia.Set(key, value)
	}
	ia.Set("flavor", serverlessInitFlavor)
}

// Submit enqueues an inventory payload synchronously.
func Submit(ia inventoryagent.Component, conf configmodel.Reader) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	ia.Submit()
	// The enqueued payload reports startup; switch to periodic for the
	// metadata runner's later submissions.
	ia.Set("report_reason", reportReasonPeriodic)
}

// buildFields flattens the per-platform inventory data and process-level
// serverless context into the (unprefixed) agent_metadata keys. env, service,
// and version come from the tag map so they match the values this container
// tags its telemetry with; site is not a tag, so it comes from the config.
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

	// wrapped_command is opt-in because redaction is best effort; it is set to
	// nil when disabled to clear any value cached by the component.
	fields["wrapped_command"] = nil
	if conf.GetBool("serverless.inventory_wrapped_command_enabled") && len(wrappedCommand) > 0 {
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

func deploymentModel(modeConf mode.Conf) string {
	if modeConf.SidecarMode {
		return "sidecar"
	}
	return "in-container"
}
