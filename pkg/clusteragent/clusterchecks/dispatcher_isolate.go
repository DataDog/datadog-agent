// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func (d *dispatcher) isolateCheck(isolateCheckID string) types.IsolateResponse {
	// Update stats prior to starting isolate to ensure all checks are accounted for
	d.updateRunnersStats()
	currentDistribution := d.currentDistribution()

	// If there is only one runner, we cannot isolate the check
	if len(currentDistribution.runnerWorkers()) == 1 {
		return types.IsolateResponse{
			CheckID:    isolateCheckID,
			CheckNode:  "",
			IsIsolated: false,
			Reason:     "No other runners available",
		}
	}

	// Distribution is keyed by digest; translate the caller's checkID.
	d.store.RLock()
	isolateDigest, isolateKnown := d.store.idToDigest[checkid.ID(isolateCheckID)]
	d.store.RUnlock()

	isolateNode := ""
	if isolateKnown {
		isolateNode = currentDistribution.runnerForConfig(isolateDigest)
	}
	if isolateNode == "" {
		return types.IsolateResponse{
			CheckID:    isolateCheckID,
			CheckNode:  "",
			IsIsolated: false,
			Reason:     "Unable to find check",
		}
	}

	proposedDistribution := newConfigsDistribution(currentDistribution.runnerWorkers(), pkgconfigsetup.Datadog().GetBool("cluster_checks.stickiness_enabled"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_factor"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_upper_limit"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_lower_limit"))

	for _, digest := range currentDistribution.configsSortedByWorkersNeeded() {
		if digest == isolateDigest {
			// Keep the config to be isolated on its current runner
			continue
		}

		config := currentDistribution.Configs[digest]
		proposedDistribution.addToLeastBusy(
			digest,
			config.CheckName,
			config.WorkersNeeded,
			config.Runner,
			isolateNode,
			false,
		)
	}

	d.applyDistribution(proposedDistribution, currentDistribution)
	return types.IsolateResponse{
		CheckID:    isolateCheckID,
		CheckNode:  isolateNode,
		IsIsolated: true,
	}
}
