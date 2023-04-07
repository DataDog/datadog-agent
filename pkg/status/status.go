// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

var timeFormat = "2006-01-02 15:04:05.999 MST"

// GetStatus grabs the status from expvar and puts it into a map
func GetStatus() (map[string]interface{}, error) {
	stats, err := getCommonStatus()
	if err != nil {
		return nil, err
	}

	stats["config"] = getPartialConfig()
	metadata := stats["metadata"].(*host.Payload)
	hostTags := make([]string, 0, len(metadata.HostTags.System)+len(metadata.HostTags.GoogleCloudPlatform))
	hostTags = append(hostTags, metadata.HostTags.System...)
	hostTags = append(hostTags, metadata.HostTags.GoogleCloudPlatform...)
	stats["hostTags"] = hostTags

	pythonVersion := host.GetPythonVersion()
	stats["python_version"] = strings.Split(pythonVersion, " ")[0]
	stats["hostinfo"] = host.GetStatusInformation()

	stats["JMXStatus"] = GetJMXStatus()
	stats["JMXStartupError"] = GetJMXStartupError()

	stats["logsStats"] = logs.GetStatus()

	stats["otlp"] = GetOTLPStatus()

	endpointsInfos, err := getEndpointsInfos()
	if endpointsInfos != nil && err == nil {
		stats["endpointsInfos"] = endpointsInfos
	} else {
		stats["endpointsInfos"] = nil
	}

	if config.Datadog.GetBool("cluster_agent.enabled") {
		stats["clusterAgentStatus"] = getDCAStatus()
	}

	if config.SystemProbe.GetBool("system_probe_config.enabled") {
		stats["systemProbeStats"] = GetSystemProbeStats(config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	}

	stats["processAgentStatus"] = GetProcessAgentStatus()

	if !config.Datadog.GetBool("no_proxy_nonexact_match") {
		httputils.NoProxyMapMutex.Lock()
		stats["TransportWarnings"] = len(httputils.NoProxyIgnoredWarningMap)+len(httputils.NoProxyUsedInFuture)+len(httputils.NoProxyChanged) > 0
		stats["NoProxyIgnoredWarningMap"] = httputils.NoProxyIgnoredWarningMap
		stats["NoProxyUsedInFuture"] = httputils.NoProxyUsedInFuture
		stats["NoProxyChanged"] = httputils.NoProxyChanged
		httputils.NoProxyMapMutex.Unlock()
	}

	if config.IsContainerized() {
		stats["adEnabledFeatures"] = config.GetDetectedFeatures()
		if common.AC != nil {
			stats["adConfigErrors"] = common.AC.GetAutodiscoveryErrors()
		}
		stats["filterErrors"] = containers.GetFilterErrors()
	}

	return stats, nil
}

// GetAndFormatStatus gets and formats the status all in one go
func GetAndFormatStatus() ([]byte, error) {
	s, err := GetStatus()
	if err != nil {
		return nil, err
	}

	statusJSON, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	st, err := FormatStatus(statusJSON)
	if err != nil {
		return nil, err
	}

	return []byte(st), nil
}

// GetCheckStatusJSON gets the status of a single check as JSON
func GetCheckStatusJSON(c check.Check, cs *check.Stats) ([]byte, error) {
	s, err := GetStatus()
	if err != nil {
		return nil, err
	}
	checks := s["runnerStats"].(map[string]interface{})["Checks"].(map[string]interface{})
	checks[c.String()] = make(map[check.ID]interface{})
	checks[c.String()].(map[check.ID]interface{})[c.ID()] = cs

	statusJSON, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	return statusJSON, nil
}

// GetCheckStatus gets the status of a single check as human-readable text
func GetCheckStatus(c check.Check, cs *check.Stats) ([]byte, error) {
	statusJSON, err := GetCheckStatusJSON(c, cs)
	if err != nil {
		return nil, err
	}

	st, err := renderCheckStats(statusJSON, c.String())
	if err != nil {
		return nil, err
	}

	return []byte(st), nil
}

// GetDCAStatus grabs the status from expvar and puts it into a map
func GetDCAStatus() (map[string]interface{}, error) {
	stats, err := getCommonStatus()
	if err != nil {
		return nil, err
	}

	stats["config"] = getDCAPartialConfig()
	stats["leaderelection"] = getLeaderElectionDetails()

	stats["logsStats"] = logs.GetStatus()

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
	s, err := GetDCAStatus()
	if err != nil {
		log.Infof("Error while getting status %q", err)
		return nil, err
	}
	statusJSON, err := json.Marshal(s)
	if err != nil {
		log.Infof("Error while marshalling %q", err)
		return nil, err
	}
	st, err := FormatDCAStatus(statusJSON)
	if err != nil {
		log.Infof("Error formatting the status %q", err)
		return nil, err
	}
	return []byte(st), nil
}

// GetAndFormatSecurityAgentStatus gets and formats the security agent status
func GetAndFormatSecurityAgentStatus(runtimeStatus, complianceStatus map[string]interface{}) ([]byte, error) {
	s, err := GetStatus()
	if err != nil {
		return nil, err
	}
	s["runtimeSecurityStatus"] = runtimeStatus
	s["complianceStatus"] = complianceStatus

	statusJSON, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	st, err := FormatSecurityAgentStatus(statusJSON)
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

	forwarderStorageMaxSizeInBytes := config.Datadog.GetInt("forwarder_storage_max_size_in_bytes")
	if forwarderStorageMaxSizeInBytes > 0 {
		conf["forwarder_storage_max_size_in_bytes"] = strconv.Itoa(forwarderStorageMaxSizeInBytes)
	}
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

// getCommonStatus grabs the status from expvar and puts it into a map.
// It gets the status elements common to all Agent flavors.
func getCommonStatus() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats, err := expvarStats(stats)
	if err != nil {
		log.Errorf("Error Getting ExpVar Stats: %v", err)
	}

	stats["version"] = version.AgentVersion
	stats["flavor"] = flavor.GetFlavor()
	hostnameData, err := hostname.GetWithProvider(context.TODO())

	if err != nil {
		log.Errorf("Error grabbing hostname for status: %v", err)
		stats["metadata"] = host.GetPayloadFromCache(context.TODO(), hostname.Data{Hostname: "unknown", Provider: "unknown"})
	} else {
		stats["metadata"] = host.GetPayloadFromCache(context.TODO(), hostnameData)
	}

	stats["conf_file"] = config.Datadog.ConfigFileUsed()
	stats["pid"] = os.Getpid()
	stats["go_version"] = runtime.Version()
	stats["agent_start_nano"] = config.StartTime.UnixNano()
	stats["build_arch"] = runtime.GOARCH
	now := time.Now()
	stats["time_nano"] = now.UnixNano()

	return stats, nil
}

func expvarStats(stats map[string]interface{}) (map[string]interface{}, error) {
	var err error
	forwarderStatsJSON := []byte(expvar.Get("forwarder").String())
	forwarderStats := make(map[string]interface{})
	json.Unmarshal(forwarderStatsJSON, &forwarderStats) //nolint:errcheck
	stats["forwarderStats"] = forwarderStats

	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats) //nolint:errcheck
	stats["runnerStats"] = runnerStats

	autoConfigStatsJSON := []byte(expvar.Get("autoconfig").String())
	autoConfigStats := make(map[string]interface{})
	json.Unmarshal(autoConfigStatsJSON, &autoConfigStats) //nolint:errcheck
	stats["autoConfigStats"] = autoConfigStats

	checkSchedulerStatsJSON := []byte(expvar.Get("CheckScheduler").String())
	checkSchedulerStats := make(map[string]interface{})
	json.Unmarshal(checkSchedulerStatsJSON, &checkSchedulerStats) //nolint:errcheck
	stats["checkSchedulerStats"] = checkSchedulerStats

	aggregatorStatsJSON := []byte(expvar.Get("aggregator").String())
	aggregatorStats := make(map[string]interface{})
	json.Unmarshal(aggregatorStatsJSON, &aggregatorStats) //nolint:errcheck
	stats["aggregatorStats"] = aggregatorStats
	s, err := check.TranslateEventPlatformEventTypes(stats["aggregatorStats"])
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

	pyLoaderData := expvar.Get("pyLoader")
	if pyLoaderData != nil {
		pyLoaderStatsJSON := []byte(pyLoaderData.String())
		pyLoaderStats := make(map[string]interface{})
		json.Unmarshal(pyLoaderStatsJSON, &pyLoaderStats) //nolint:errcheck
		stats["pyLoaderStats"] = pyLoaderStats
	} else {
		stats["pyLoaderStats"] = nil
	}

	pythonInitData := expvar.Get("pythonInit")
	if pythonInitData != nil {
		pythonInitJSON := []byte(pythonInitData.String())
		pythonInit := make(map[string]interface{})
		json.Unmarshal(pythonInitJSON, &pythonInit) //nolint:errcheck
		stats["pythonInit"] = pythonInit
	} else {
		stats["pythonInit"] = nil
	}

	hostnameStatsJSON := []byte(expvar.Get("hostname").String())
	hostnameStats := make(map[string]interface{})
	json.Unmarshal(hostnameStatsJSON, &hostnameStats) //nolint:errcheck
	stats["hostnameStats"] = hostnameStats

	ntpOffset := expvar.Get("ntpOffset")
	if ntpOffset != nil && ntpOffset.String() != "" {
		stats["ntpOffset"], err = strconv.ParseFloat(expvar.Get("ntpOffset").String(), 64)
	}

	inventories := expvar.Get("inventories")
	var inventoriesStats map[string]interface{}
	if inventories != nil {
		inventoriesStatsJSON := []byte(inventories.String())
		json.Unmarshal(inventoriesStatsJSON, &inventoriesStats) //nolint:errcheck
	}

	checkMetadata := map[string]map[string]string{}
	if data, ok := inventoriesStats["check_metadata"]; ok {
		for _, instances := range data.(map[string]interface{}) {
			for _, instance := range instances.([]interface{}) {
				metadata := map[string]string{}
				checkHash := ""
				for k, v := range instance.(map[string]interface{}) {
					if vStr, ok := v.(string); ok {
						if k == "config.hash" {
							checkHash = vStr
						} else if k != "config.provider" {
							metadata[k] = vStr
						}
					}
				}
				if checkHash != "" && len(metadata) != 0 {
					checkMetadata[checkHash] = metadata
				}
			}
		}
	}
	stats["inventories"] = checkMetadata
	if data, ok := inventoriesStats["agent_metadata"]; ok {
		stats["agent_metadata"] = data
	} else {
		stats["agent_metadata"] = map[string]string{}
	}

	stats["snmpTrapsStats"] = traps.GetStatus()

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
