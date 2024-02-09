// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"

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

	isolateNode := currentDistribution.runnerForCheck(isolateCheckID)
	if isolateNode == "" {
		return types.IsolateResponse{
			CheckID:    isolateCheckID,
			CheckNode:  "",
			IsIsolated: false,
			Reason:     "Unable to find check",
		}
	}

	proposedDistribution := newChecksDistribution(currentDistribution.runnerWorkers())

	for _, checkID := range currentDistribution.checksSortedByWorkersNeeded() {
		if checkID == isolateCheckID {
			// Keep the check to be isolated on its current runner
			continue
		}

		workersNeededForCheck := currentDistribution.workersNeededForCheck(checkID)
		runnerForCheck := currentDistribution.runnerForCheck(checkID)

		proposedDistribution.addToLeastBusy(
			checkID,
			workersNeededForCheck,
			runnerForCheck,
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
