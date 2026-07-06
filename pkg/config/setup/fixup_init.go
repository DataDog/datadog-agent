// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup defines the configuration of the agent
package setup

import (
	"os"
	"runtime"

	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func fixupContainerSyspath(config pkgconfigmodel.Config) {
	procfsPathDefault := ""
	containerProcRootDefault := ""
	containerCgroupRootDefault := ""

	if pkgconfigenv.IsContainerized() {
		// In serverless-containerized environments (e.g Fargate)
		// it's impossible to mount host volumes.
		// Make sure the host paths exist before setting-up the default values.
		// Fallback to the container paths if host paths aren't mounted.
		if pathExists("/host/proc") {
			procfsPathDefault = "/host/proc"
			containerProcRootDefault = "/host/proc"

			// Used by some librairies (like gopsutil)
			if v := os.Getenv("HOST_PROC"); v == "" {
				os.Setenv("HOST_PROC", "/host/proc")
			}
		} else {
			procfsPathDefault = "/proc"
			containerProcRootDefault = "/proc"
		}
		if pathExists("/host/sys/fs/cgroup/") {
			containerCgroupRootDefault = "/host/sys/fs/cgroup/"
		} else {
			containerCgroupRootDefault = "/sys/fs/cgroup/"
		}
	} else {
		containerProcRootDefault = "/proc"
		// for amazon linux the cgroup directory on host is /cgroup/
		// we pick memory.stat to make sure it exists and not empty
		if _, err := os.Stat("/cgroup/memory/memory.stat"); !os.IsNotExist(err) {
			containerCgroupRootDefault = "/cgroup/"
		} else {
			containerCgroupRootDefault = "/sys/fs/cgroup/"
		}
	}

	config.Set("procfs_path", procfsPathDefault, pkgconfigmodel.SourceDefault)
	config.Set("container_proc_root", containerProcRootDefault, pkgconfigmodel.SourceDefault)
	config.Set("container_cgroup_root", containerCgroupRootDefault, pkgconfigmodel.SourceDefault)
}

func fixupLogsAgent(config pkgconfigmodel.Config) {
	// Number of logs pipeline instances. Defaults to number of logical CPU cores as defined by GOMAXPROCS or 4, whichever is lower.
	maxProcs := runtime.GOMAXPROCS(0)
	if maxProcs < 4 {
		config.Set("logs_config.pipelines", maxProcs, pkgconfigmodel.SourceDefault)
	}
}

// always called, for both full-agent and serverless-init, after declaring settings
func fixupInitCommonConfigComponents(config pkgconfigmodel.Config) {
	pkgconfigmodel.AddOverrideFunc(FleetConfigOverride)
	fixupContainerSyspath(config)
	fixupLogsAgent(config)
	pkgconfigmodel.AddOverrideFunc(toggleDefaultPayloads)
	pkgconfigmodel.AddOverrideFunc(applyInfrastructureModeOverrides)
	pkgconfigmodel.AddOverrideFunc(ApplyUseDogstatsdSuppression)
	pkgconfigmodel.AddOverrideFunc(ComputeDataPlaneStopTimeout)
}

// called only for full-agent, NOT serverless-init, after declaring settings
func fixupInitFullAgentOnlyComponents(_ pkgconfigmodel.Config) {
	pkgconfigmodel.AddOverrideFunc(sanitizeExternalMetricsProviderChunkSize)
}

// called only for system-probe, after declaring settings
func fixupInitSystemProbe(_ pkgconfigmodel.Config) {
}
