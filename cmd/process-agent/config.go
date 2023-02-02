package main

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getChecks(sysCfg *sysconfig.Config, canAccessContainers bool) (checkCfg []checks.Check) {
	rtChecksEnabled := !ddconfig.Datadog.GetBool("process_config.disable_realtime_checks")
	if ddconfig.Datadog.GetBool("process_config.process_collection.enabled") {
		checkCfg = append(checkCfg, checks.NewProcessCheck())
	} else {
		if ddconfig.Datadog.GetBool("process_config.container_collection.enabled") && canAccessContainers {
			checkCfg = append(checkCfg, checks.NewContainerCheck())
			if rtChecksEnabled {
				checkCfg = append(checkCfg, checks.NewRTContainerCheck())
			}
		} else if !canAccessContainers {
			_ = log.Warn("Disabled container check because no container environment detected (see list of detected features in `agent status`)")
		}

		if ddconfig.Datadog.GetBool("process_config.process_discovery.enabled") {
			if ddconfig.IsECSFargate() {
				log.Debug("Process discovery is not supported on ECS Fargate")
			} else {
				checkCfg = append(checkCfg, checks.NewProcessDiscoveryCheck())
			}
		}
	}

	if ddconfig.Datadog.GetBool("process_config.event_collection.enabled") {
		checkCfg = append(checkCfg, checks.NewProcessEventsCheck())
	}

	if isOrchestratorCheckEnabled() {
		checkCfg = append(checkCfg, checks.NewPodCheck())
	}

	if sysCfg.Enabled {
		if _, ok := sysCfg.EnabledModules[sysconfig.NetworkTracerModule]; ok {
			checkCfg = append(checkCfg, checks.NewConnectionsCheck())
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
