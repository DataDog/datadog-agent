// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status implements the status of the agent
package status

import (
	"encoding/json"
	"expvar"
	"strings"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/status/apm"
	"github.com/DataDog/datadog-agent/pkg/status/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/status/clusteragent"
	commonStatus "github.com/DataDog/datadog-agent/pkg/status/common"
	"github.com/DataDog/datadog-agent/pkg/status/endpoints"
	"github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/status/otlp"
	"github.com/DataDog/datadog-agent/pkg/status/processagent"
	"github.com/DataDog/datadog-agent/pkg/status/remoteconfiguration"
	"github.com/DataDog/datadog-agent/pkg/status/render"
	"github.com/DataDog/datadog-agent/pkg/status/systemprobe"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// GetStatus grabs the status from expvar and puts it into a map
func GetStatus(verbose bool, invAgent inventoryagent.Component) (map[string]interface{}, error) {
	stats, err := commonStatus.GetStatus(invAgent)
	if err != nil {
		return nil, err
	}
	stats["verbose"] = verbose
	stats["config"] = getPartialConfig()
	metadata := stats["metadata"].(*hostMetadataUtils.Payload)
	hostTags := make([]string, 0, len(metadata.HostTags.System)+len(metadata.HostTags.GoogleCloudPlatform))
	hostTags = append(hostTags, metadata.HostTags.System...)
	hostTags = append(hostTags, metadata.HostTags.GoogleCloudPlatform...)
	stats["hostTags"] = hostTags

	pythonVersion := python.GetPythonVersion()
	stats["python_version"] = strings.Split(pythonVersion, " ")[0]
	stats["hostinfo"] = hostMetadataUtils.GetInformation()

	jmx.PopulateStatus(stats)

	stats["logsStats"] = logsStatus.Get(verbose)

	otlp.PopulateStatus(stats)

	endpoints.PopulateStatus(stats)

	if config.Datadog.GetBool("cluster_agent.enabled") || config.Datadog.GetBool("cluster_checks.enabled") {
		stats["clusterAgentStatus"] = clusteragent.GetDCAStatus()
	}

	if config.SystemProbe.GetBool("system_probe_config.enabled") {
		stats["systemProbeStats"] = systemprobe.GetStatus(config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	}

	stats["processAgentStatus"] = processagent.GetStatus()
	stats["apmStats"] = apm.GetAPMStatus()

	if !config.Datadog.GetBool("no_proxy_nonexact_match") {
		stats["TransportWarnings"] = httputils.GetNumberOfWarnings() > 0
		stats["NoProxyIgnoredWarningMap"] = httputils.GetProxyIgnoredWarnings()
		stats["NoProxyUsedInFuture"] = httputils.GetProxyUsedInFutureWarnings()
		stats["NoProxyChanged"] = httputils.GetProxyIgnoredWarnings()
	}

	if config.IsContainerized() {
		autodiscovery.PopulateStatus(stats)
	}

	remoteconfiguration.PopulateStatus(stats)
	return stats, nil
}

// GetAndFormatStatus gets and formats the status all in one go
func GetAndFormatStatus(invAgent inventoryagent.Component) ([]byte, error) {
	s, err := GetStatus(true, invAgent)
	if err != nil {
		return nil, err
	}

	statusJSON, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	st, err := render.FormatStatus(statusJSON)
	if err != nil {
		return nil, err
	}

	return []byte(st), nil
}

// GetAndFormatSecurityAgentStatus gets and formats the security agent status
func GetAndFormatSecurityAgentStatus(runtimeStatus, complianceStatus map[string]interface{}) ([]byte, error) {
	// inventory metadata is not enabled in the security agent, we pass nil to GetStatus
	s, err := GetStatus(true, nil)
	if err != nil {
		return nil, err
	}
	s["runtimeSecurityStatus"] = runtimeStatus
	s["complianceStatus"] = complianceStatus

	statusJSON, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	st, err := render.FormatSecurityAgentStatus(statusJSON)
	if err != nil {
		return nil, err
	}

	return []byte(st), nil
}

// getPartialConfig returns config parameters of interest for the status page
func getPartialConfig() map[string]string {
	conf := make(map[string]string)
	conf["log_file"] = config.Datadog.GetString("log_file")
	conf["log_level"] = config.Datadog.GetString("log_level")
	conf["confd_path"] = config.Datadog.GetString("confd_path")
	conf["additional_checksd"] = config.Datadog.GetString("additional_checksd")

	conf["fips_enabled"] = config.Datadog.GetString("fips.enabled")
	conf["fips_local_address"] = config.Datadog.GetString("fips.local_address")
	conf["fips_port_range_start"] = config.Datadog.GetString("fips.port_range_start")

	return conf
}

// GetExpvarRunnerStats grabs the status of the runner from expvar
// and puts it into a CLCChecks struct
func GetExpvarRunnerStats() (CLCChecks, error) {
	runnerStatsJSON := []byte(expvar.Get("runner").String())
	return convertExpvarRunnerStats(runnerStatsJSON)
}

func convertExpvarRunnerStats(inputJSON []byte) (CLCChecks, error) {
	runnerStats := CLCChecks{}
	err := json.Unmarshal(inputJSON, &runnerStats)
	return runnerStats, err
}
