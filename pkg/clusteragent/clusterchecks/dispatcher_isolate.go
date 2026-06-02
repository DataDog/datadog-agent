// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
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
	isolateDigest := d.store.idToDigest[checkid.ID(isolateCheckID)]
	d.store.RUnlock()

	isolateNode := currentDistribution.runnerForCheck(isolateDigest)
	if isolateNode == "" {
		return types.IsolateResponse{
			CheckID:    isolateCheckID,
			CheckNode:  "",
			IsIsolated: false,
			Reason:     "Unable to find check",
		}
	}

	proposedDistribution := newChecksDistribution(currentDistribution.runnerWorkers())

	for _, digest := range currentDistribution.checksSortedByWorkersNeeded() {
		if digest == isolateDigest {
			// Keep the config to be isolated on its current runner
			continue
		}

		check := currentDistribution.Checks[digest]
		proposedDistribution.addToLeastBusy(
			digest,
			check.CheckName,
			check.WorkersNeeded,
			check.Runner,
			isolateNode,
		)
	}

	d.applyDistribution(proposedDistribution, currentDistribution)
	return types.IsolateResponse{
		CheckID:    isolateCheckID,
		CheckNode:  isolateNode,
		IsIsolated: true,
	}
}
