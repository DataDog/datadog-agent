package main

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
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
		}
		if ddconfig.Datadog.GetBool("process_config.process_discovery.enabled") {
			checkCfg = append(checkCfg, checks.ProcessDiscovery)
		}
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
		if _, ok := sysCfg.EnabledModules[sysconfig.ProcessModule]; ok {
			checks.Process.SysprobeProcessModuleEnabled = true
		}
		if _, ok := sysCfg.EnabledModules[sysconfig.NetworkTracerModule]; ok {
			checkCfg = append(checkCfg, checks.Connections)
		}
	}

	return
}
