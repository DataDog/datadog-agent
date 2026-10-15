// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package serverlessinitimpl

import (
	"strings"

	serverlessinit "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
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

// buildFields flattens the per-platform inventory data and process-level
// serverless context into the (unprefixed) agent_metadata keys. env, service,
// and version come from the tag map so they match the values this container
// tags its telemetry with; site is not a tag, so it comes from the config.
func buildFields(inv serverlessinit.InventoryData, params serverlessinit.Params, conf configmodel.Reader) map[string]interface{} {
	wrappedCommand := params.WrappedCommand
	if params.Sidecar {
		wrappedCommand = nil
	}

	fields := map[string]interface{}{
		"serverless_init_version": serverlessTags.GetExtensionVersion(),
		"agent_version_base":      version.AgentVersion,
		"agent_commit":            version.Commit,
		"report_reason":           reportReasonStartup,
		"flavor":                  serverlessInitFlavor,

		"resource_id":        inv.ResourceID,
		"resource_name":      inv.ResourceName,
		"workload_type":      inv.WorkloadType,
		"parent_resource_id": inv.ParentResourceID,

		"region":                inv.Region,
		"gcp_project_id":        inv.GCPProjectID,
		"aws_account_id":        inv.AWSAccountID,
		"azure_subscription_id": inv.AzureSubscriptionID,
		"azure_resource_group":  inv.AzureResourceGroup,

		"deployment_model": deploymentModel(params.Sidecar),
		"runtime":          resolveRuntime(inv.RuntimeCandidates, wrappedCommand),

		"dd_env":     params.Tags["env"],
		"dd_site":    conf.GetString("site"),
		"dd_version": params.Tags["version"],
		"dd_service": params.Tags["service"],
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

func deploymentModel(sidecar bool) string {
	if sidecar {
		return "sidecar"
	}
	return "in-container"
}
