// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"net/url"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getChecks(sysCfg *sysconfig.Config, canAccessContainers bool) (checkCfg []checks.Check) {
	rtChecksEnabled := !ddconfig.Datadog.GetBool("process_config.disable_realtime_checks")
	if ddconfig.Datadog.GetBool("process_config.process_collection.enabled") {
		checkCfg = append(checkCfg, checks.Process)
	} else {
		if ddconfig.Datadog.GetBool("process_config.container_collection.enabled") && canAccessContainers {
			checkCfg = append(checkCfg, checks.Container)
			if rtChecksEnabled {
				checkCfg = append(checkCfg, checks.RTContainer)
			}
		} else if !canAccessContainers {
			_ = log.Warn("Disabled container check because no container environment detected (see list of detected features in `agent status`)")
		}

		if ddconfig.Datadog.GetBool("process_config.process_discovery.enabled") {
			if ddconfig.IsECSFargate() {
				log.Debug("Process discovery is not supported on ECS Fargate")
			} else {
				checkCfg = append(checkCfg, checks.ProcessDiscovery)
			}
		}
	}

	if ddconfig.Datadog.GetBool("process_config.event_collection.enabled") {
		checkCfg = append(checkCfg, checks.ProcessEvents)
	}

	if isOrchestratorCheckEnabled() {
		checkCfg = append(checkCfg, checks.Pod)
	}

	if sysCfg.Enabled {
		// If the sysprobe module is enabled, the process check can call out to the sysprobe for privileged stats
		_, checks.Process.SysprobeProcessModuleEnabled = sysCfg.EnabledModules[sysconfig.ProcessModule]

		if _, ok := sysCfg.EnabledModules[sysconfig.NetworkTracerModule]; ok {
			checkCfg = append(checkCfg, checks.Connections)
		}
	}

	return
}

func isOrchestratorCheckEnabled() bool {
	// activate the pod collection if enabled and we have the cluster name set
	orchestratorEnabled, kubeClusterName := oconfig.IsOrchestratorEnabled()
	if !orchestratorEnabled {
		return false
	}

	if kubeClusterName == "" {
		_ = log.Warnf("Failed to auto-detect a Kubernetes cluster name. Pod collection will not start. To fix this, set it manually via the cluster_name config option")
		return false
	}
	return true
}

func getAPIEndpoints() (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys("https://process.", "process_config.process_dd_url", "process_config.additional_endpoints")
}

func getEventsAPIEndpoints() (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys("https://process-events.", "process_config.events_dd_url", "process_config.events_additional_endpoints")
}

func getAPIEndpointsWithKeys(prefix, defaultEpKey, additionalEpsKey string) (eps []apicfg.Endpoint, err error) {
	// Setup main endpoint
	mainEndpointURL, err := url.Parse(ddconfig.GetMainEndpoint(prefix, defaultEpKey))
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %s", defaultEpKey, err)
	}
	eps = append(eps, apicfg.Endpoint{
		APIKey:   ddconfig.SanitizeAPIKey(ddconfig.Datadog.GetString("api_key")),
		Endpoint: mainEndpointURL,
	})

	// Optional additional pairs of endpoint_url => []apiKeys to submit to other locations.
	for endpointURL, apiKeys := range ddconfig.Datadog.GetStringMapStringSlice(additionalEpsKey) {
		u, err := url.Parse(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("invalid %s url '%s': %s", additionalEpsKey, endpointURL, err)
		}
		for _, k := range apiKeys {
			eps = append(eps, apicfg.Endpoint{
				APIKey:   ddconfig.SanitizeAPIKey(k),
				Endpoint: u,
			})
		}
	}
	return
}
