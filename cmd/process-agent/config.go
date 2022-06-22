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

func getChecks(sysCfg *sysconfig.Config, oCfg *oconfig.OrchestratorConfig, canAccessContainers bool) (checkCfg []checks.Check) {
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

	// activate the pod collection if enabled and we have the cluster name set
	if oCfg.OrchestrationCollectionEnabled {
		if oCfg.KubeClusterName != "" {
			checkCfg = append(checkCfg, checks.Pod)
		} else {
			_ = log.Warnf("Failed to auto-detect a Kubernetes cluster name. Pod collection will not start. To fix this, set it manually via the cluster_name config option")
		}
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

func getAPIEndpoints() (eps []apicfg.Endpoint, err error) {
	// Setup main endpoint
	mainEndpointURL, err := url.Parse(ddconfig.GetMainEndpoint("https://process.", "process_config.process_dd_url"))
	if err != nil {
		return nil, fmt.Errorf("error parsing process_dd_url: %s", err)
	}
	eps = append(eps, apicfg.Endpoint{
		APIKey:   ddconfig.SanitizeAPIKey(ddconfig.Datadog.GetString("api_key")),
		Endpoint: mainEndpointURL,
	})

	// Optional additional pairs of endpoint_url => []apiKeys to submit to other locations.
	for endpointURL, apiKeys := range ddconfig.Datadog.GetStringMapStringSlice("process_config.additional_endpoints") {
		u, err := url.Parse(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("invalid additional endpoint url '%s': %s", endpointURL, err)
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
