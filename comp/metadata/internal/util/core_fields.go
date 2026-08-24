// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// installinfoGet is a package var so tests can stub it.
var installinfoGet = installinfo.Get

// PopulateCoreFields sets the subset of agent-metadata fields required for a
// payload to be recognized and processed as an agent-metadata payload by the
// inventory pipeline. It populates only fields with no cross-process
// dependency; callers layer their own component-specific and config-fetched
// fields on top.
//
// It lives here, in the shared inventory util package, so both the core-agent
// inventoryagent component and the serverlessinventory component have one
// source of truth without the serverless component importing the core-agent
// component's cross-process machinery.
//
// flavor is a parameter rather than flavor.GetFlavor() so a caller can emit a
// payload flavor (e.g. "serverless-init") without changing the process-global
// flavor, which the aggregator captures once for its heartbeat metric.
//
// hostnameSource is only recorded when non-empty: callers with no resolvable
// host (serverless) pass "" and the key is omitted, matching the core agent's
// conditional behavior.
func PopulateCoreFields(data map[string]interface{}, conf model.Reader, flavor string, hostnameSource string) {
	tool := "undefined"
	toolVersion := ""
	installerVersion := ""

	install, err := installinfoGet(conf)
	if err == nil {
		tool = install.Tool
		toolVersion = install.ToolVersion
		installerVersion = install.InstallerVersion
	}
	data["install_method_tool"] = tool
	data["install_method_tool_version"] = toolVersion
	data["install_method_installer_version"] = installerVersion

	if hostnameSource != "" {
		data["hostname_source"] = hostnameSource
	}

	data["agent_version"] = version.AgentVersion
	data["package_version"] = version.AgentPackageVersion
	data["agent_startup_time_ms"] = conf.StartTime().UnixMilli()
	data["flavor"] = flavor

	infraMode, _ := scrubber.ScrubString(conf.GetString("infrastructure_mode"))
	// fleet-automation: This validation should be done by the Config once we have such mechanism
	if !slices.Contains([]string{"full", "end_user_device", "basic", "cloud_cost_only", "none"}, infraMode) {
		log.Warnf("invalid value for 'infrastructure_mode': '%s' (defaulting to 'full')", infraMode)
		infraMode = "full"
	}
	data["infrastructure_mode"] = infraMode
}
