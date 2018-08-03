// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import (
	"encoding/json"
	"expvar"
	"os"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/render"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetStatus returns status info for the Datadog Agent.
func GetStatus() (map[string]interface{}, error) {
	status := make(map[string]interface{})
	status, err := expvarStats(status)
	if err != nil {
		log.Errorf("Error Getting ExpVar Stats: %v", err)
	}
	status["version"] = version.AgentVersion
	hostname, err := util.GetHostname()
	if err != nil {
		log.Errorf("Error grabbing hostname for status: %v", err)
		status["metadata"] = host.GetPayloadFromCache("unknown")
	} else {
		status["metadata"] = host.GetPayloadFromCache(hostname)
	}

	status["config"] = getPartialConfig()
	status["conf_file"] = config.Datadog.ConfigFileUsed()

	platformPayload, err := getPlatformPayload()
	if err != nil {
		return nil, err
	}
	status["pid"] = os.Getpid()
	status["platform"] = platformPayload
	status["hostinfo"] = host.GetStatusInformation()
	now := time.Now()
	status["time"] = now.Format(render.TimeFormat)

	status["JMXStatus"] = GetJMXStatus()

	status["logsStats"] = logs.GetStatus()

	if config.Datadog.GetBool("cluster_agent.enabled") {
		status["clusterAgentStatus"] = clusteragent.GetStatus()
	}

	return status, nil
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

// GetCheckStatus gets the status of a single check
func GetCheckStatus(c check.Check, cs *check.Stats) ([]byte, error) {
	s, err := GetStatus()
	if err != nil {
		return nil, err
	}
	checks := s["runnerStats"].(map[string]interface{})["Checks"]
	checks.(map[string]interface{})[c.String()] = cs

	statusJSON, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	st, err := renderCheckStats(statusJSON, c.String())
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
	return conf
}

func expvarStats(stats map[string]interface{}) (map[string]interface{}, error) {
	stats["forwarderStats"] = forwarder.GetStatus()
	stats["runnerStats"] = collector.GetRunnerStatus()
	stats["autoConfigStats"] = autodiscovery.GetAutoConfigStatus()
	stats["checkSchedulerStats"] = collector.GetCheckSchedulerStatus()

	aggregatorStatsJSON := []byte(expvar.Get("aggregator").String())
	aggregatorStats := make(map[string]interface{})
	json.Unmarshal(aggregatorStatsJSON, &aggregatorStats)
	stats["aggregatorStats"] = aggregatorStats

	pyLoaderData := expvar.Get("pyLoader")
	if pyLoaderData != nil {
		pyLoaderStatsJSON := []byte(pyLoaderData.String())
		pyLoaderStats := make(map[string]interface{})
		json.Unmarshal(pyLoaderStatsJSON, &pyLoaderStats)
		stats["pyLoaderStats"] = pyLoaderStats
	} else {
		stats["pyLoaderStats"] = nil
	}

	var err error
	if expvar.Get("ntpOffset").String() != "" {
		stats["ntpOffset"], err = strconv.ParseFloat(expvar.Get("ntpOffset").String(), 64)
	}

	return stats, err
}
