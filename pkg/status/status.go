// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(ASC) Fix revive linter
package status

import (
	"context"
	"encoding/json"
	"expvar"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/orchestrator"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/status/apm"
	"github.com/DataDog/datadog-agent/pkg/status/collector"
	"github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/status/otlp"
	"github.com/DataDog/datadog-agent/pkg/status/render"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetStatus grabs the status from expvar and puts it into a map
func GetStatus(verbose bool, invAgent inventoryagent.Component) (map[string]interface{}, error) {
	stats, err := getCommonStatus(invAgent)
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

	endpointsInfos, err := getEndpointsInfos()
	if endpointsInfos != nil && err == nil {
		stats["endpointsInfos"] = endpointsInfos
	} else {
		stats["endpointsInfos"] = nil
	}

	if config.Datadog.GetBool("cluster_agent.enabled") || config.Datadog.GetBool("cluster_checks.enabled") {
		stats["clusterAgentStatus"] = getDCAStatus()
	}

	if config.SystemProbe.GetBool("system_probe_config.enabled") {
		stats["systemProbeStats"] = GetSystemProbeStats(config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	}

	stats["processAgentStatus"] = GetProcessAgentStatus()
	stats["apmStats"] = apm.GetAPMStatus()

	if !config.Datadog.GetBool("no_proxy_nonexact_match") {
		stats["TransportWarnings"] = httputils.GetNumberOfWarnings() > 0
		stats["NoProxyIgnoredWarningMap"] = httputils.GetProxyIgnoredWarnings()
		stats["NoProxyUsedInFuture"] = httputils.GetProxyUsedInFutureWarnings()
		stats["NoProxyChanged"] = httputils.GetProxyIgnoredWarnings()
	}

	if config.IsContainerized() {
		stats["adEnabledFeatures"] = config.GetDetectedFeatures()
		if common.AC != nil {
			stats["adConfigErrors"] = common.AC.GetAutodiscoveryErrors()
		}
		stats["filterErrors"] = containers.GetFilterErrors()
	}

	stats["remoteConfiguration"] = getRemoteConfigStatus()
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

// GetDCAStatus grabs the status from expvar and puts it into a map
func GetDCAStatus(verbose bool) (map[string]interface{}, error) {
	// inventory is not enabled for the clusteragent/DCA so we pass nil to getCommonStatus
	stats, err := getCommonStatus(nil)
	if err != nil {
		return nil, err
	}

	stats["config"] = getDCAPartialConfig()
	stats["leaderelection"] = getLeaderElectionDetails()

	if config.Datadog.GetBool("compliance_config.enabled") {
		stats["logsStats"] = logsStatus.Get(verbose)
	}

	endpointsInfos, err := getEndpointsInfos()
	if endpointsInfos != nil && err == nil {
		stats["endpointsInfos"] = endpointsInfos
	} else {
		stats["endpointsInfos"] = nil
	}

	apiCl, apiErr := apiserver.GetAPIClient()
	if apiErr != nil {
		stats["custommetrics"] = map[string]string{"Error": apiErr.Error()}
		stats["admissionWebhook"] = map[string]string{"Error": apiErr.Error()}
	} else {
		stats["custommetrics"] = custommetrics.GetStatus(apiCl.Cl)
		stats["admissionWebhook"] = admission.GetStatus(apiCl.Cl)
	}

	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		stats["externalmetrics"] = externalmetrics.GetStatus()
	} else {
		stats["externalmetrics"] = apiserver.GetStatus()
	}

	if config.Datadog.GetBool("cluster_checks.enabled") {
		cchecks, err := clusterchecks.GetStats()
		if err != nil {
			log.Errorf("Error grabbing clusterchecks stats: %s", err)
		} else {
			stats["clusterchecks"] = cchecks
		}
	}

	stats["adEnabledFeatures"] = config.GetDetectedFeatures()
	if common.AC != nil {
		stats["adConfigErrors"] = common.AC.GetAutodiscoveryErrors()
	}
	stats["filterErrors"] = containers.GetFilterErrors()

	if config.Datadog.GetBool("orchestrator_explorer.enabled") {
		if apiErr != nil {
			stats["orchestrator"] = map[string]string{"Error": apiErr.Error()}
		} else {
			orchestratorStats := orchestrator.GetStatus(context.TODO(), apiCl.Cl)
			stats["orchestrator"] = orchestratorStats
		}
	}

	return stats, nil
}

// GetAndFormatDCAStatus gets and formats the DCA status all in one go.
func GetAndFormatDCAStatus() ([]byte, error) {
	s, err := GetDCAStatus(true)
	if err != nil {
		log.Infof("Error while getting status %q", err)
		return nil, err
	}
	statusJSON, err := json.Marshal(s)
	if err != nil {
		log.Infof("Error while marshalling %q", err)
		return nil, err
	}
	st, err := render.FormatDCAStatus(statusJSON)
	if err != nil {
		log.Infof("Error formatting the status %q", err)
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

// getDCAPartialConfig returns config parameters of interest for the status page.
func getDCAPartialConfig() map[string]string {
	conf := make(map[string]string)
	conf["log_level"] = config.Datadog.GetString("log_level")
	conf["confd_path"] = config.Datadog.GetString("confd_path")
	return conf
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

func getEndpointsInfos() (map[string]interface{}, error) {
	endpoints, err := utils.GetMultipleEndpoints(config.Datadog)
	if err != nil {
		return nil, err
	}

	endpointsInfos := make(map[string]interface{})

	// obfuscate the api keys
	for endpoint, keys := range endpoints {
		for i, key := range keys {
			if len(key) > 5 {
				keys[i] = key[len(key)-5:]
			}
		}
		endpointsInfos[endpoint] = keys
	}

	return endpointsInfos, nil
}

// getRemoteConfigStatus return the current status of remote config
func getRemoteConfigStatus() map[string]interface{} {
	status := make(map[string]interface{})

	if config.IsRemoteConfigEnabled(config.Datadog) && expvar.Get("remoteConfigStatus") != nil {
		remoteConfigStatusJSON := expvar.Get("remoteConfigStatus").String()
		json.Unmarshal([]byte(remoteConfigStatusJSON), &status) //nolint:errcheck
	} else {
		if !config.Datadog.GetBool("remote_configuration.enabled") {
			status["disabledReason"] = "it is explicitly disabled in the agent configuration. (`remote_configuration.enabled: false`)"
		} else if config.Datadog.GetBool("fips.enabled") {
			status["disabledReason"] = "it is not supported when FIPS is enabled. (`fips.enabled: true`)"
		} else if config.Datadog.GetString("site") == "ddog-gov.com" {
			status["disabledReason"] = "it is not supported on GovCloud. (`site: \"ddog-gov.com\"`)"
		}
	}

	return status
}

// getCommonStatus grabs the status from expvar and puts it into a map.
// It gets the status elements common to all Agent flavors.
func getCommonStatus(invAgent inventoryagent.Component) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats, err := expvarStats(stats, invAgent)
	if err != nil {
		log.Errorf("Error Getting ExpVar Stats: %v", err)
	}

	stats["version"] = version.AgentVersion
	stats["flavor"] = flavor.GetFlavor()
	stats["metadata"] = hostMetadataUtils.GetFromCache(context.TODO(), config.Datadog)
	stats["conf_file"] = config.Datadog.ConfigFileUsed()
	stats["pid"] = os.Getpid()
	stats["go_version"] = runtime.Version()
	stats["agent_start_nano"] = config.StartTime.UnixNano()
	stats["build_arch"] = runtime.GOARCH
	now := time.Now()
	stats["time_nano"] = now.UnixNano()

	return stats, nil
}

func expvarStats(stats map[string]interface{}, invAgent inventoryagent.Component) (map[string]interface{}, error) {
	var err error
	forwarderStatsJSON := []byte(expvar.Get("forwarder").String())
	forwarderStats := make(map[string]interface{})
	json.Unmarshal(forwarderStatsJSON, &forwarderStats) //nolint:errcheck
	forwarderStorageMaxSizeInBytes := config.Datadog.GetInt("forwarder_storage_max_size_in_bytes")
	if forwarderStorageMaxSizeInBytes > 0 {
		forwarderStats["forwarder_storage_max_size_in_bytes"] = strconv.Itoa(forwarderStorageMaxSizeInBytes)
	}
	stats["forwarderStats"] = forwarderStats

	collector.PopulateStatus(stats)

	aggregatorStatsJSON := []byte(expvar.Get("aggregator").String())
	aggregatorStats := make(map[string]interface{})
	json.Unmarshal(aggregatorStatsJSON, &aggregatorStats) //nolint:errcheck
	stats["aggregatorStats"] = aggregatorStats
	s, err := checkstats.TranslateEventPlatformEventTypes(stats["aggregatorStats"])
	if err != nil {
		log.Debugf("failed to translate event platform event types in aggregatorStats: %s", err.Error())
	} else {
		stats["aggregatorStats"] = s
	}

	if expvar.Get("dogstatsd") != nil {
		dogstatsdStatsJSON := []byte(expvar.Get("dogstatsd").String())
		dogstatsdUdsStatsJSON := []byte(expvar.Get("dogstatsd-uds").String())
		dogstatsdUDPStatsJSON := []byte(expvar.Get("dogstatsd-udp").String())
		dogstatsdStats := make(map[string]interface{})
		json.Unmarshal(dogstatsdStatsJSON, &dogstatsdStats) //nolint:errcheck
		dogstatsdUdsStats := make(map[string]interface{})
		json.Unmarshal(dogstatsdUdsStatsJSON, &dogstatsdUdsStats) //nolint:errcheck
		for name, value := range dogstatsdUdsStats {
			dogstatsdStats["Uds"+name] = value
		}
		dogstatsdUDPStats := make(map[string]interface{})
		json.Unmarshal(dogstatsdUDPStatsJSON, &dogstatsdUDPStats) //nolint:errcheck
		for name, value := range dogstatsdUDPStats {
			dogstatsdStats["Udp"+name] = value
		}
		stats["dogstatsdStats"] = dogstatsdStats
	}

	hostnameStatsJSON := []byte(expvar.Get("hostname").String())
	hostnameStats := make(map[string]interface{})
	json.Unmarshal(hostnameStatsJSON, &hostnameStats) //nolint:errcheck
	stats["hostnameStats"] = hostnameStats

	ntpOffset := expvar.Get("ntpOffset")
	if ntpOffset != nil && ntpOffset.String() != "" {
		stats["ntpOffset"], err = strconv.ParseFloat(expvar.Get("ntpOffset").String(), 64)
	}

	// invAgent can be nil when generating a status page for some agent where inventory is not enabled
	// (clusteragent, security-agent, ...).
	//
	// todo: (component) remove this condition once status is a component.
	if invAgent != nil {
		stats["agent_metadata"] = invAgent.Get()
	} else {
		stats["agent_metadata"] = map[string]string{}
	}

	if traps.IsEnabled(config.Datadog) {
		stats["snmpTrapsStats"] = traps.GetStatus()
	}

	if netflowServer.IsEnabled() {
		stats["netflowStats"] = netflowServer.GetStatus()
	}

	complianceVar := expvar.Get("compliance")
	if complianceVar != nil {
		complianceStatusJSON := []byte(complianceVar.String())
		complianceStatus := make(map[string]interface{})
		json.Unmarshal(complianceStatusJSON, &complianceStatus) //nolint:errcheck
		stats["complianceChecks"] = complianceStatus["Checks"]
	} else {
		stats["complianceChecks"] = map[string]interface{}{}
	}

	return stats, err
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
