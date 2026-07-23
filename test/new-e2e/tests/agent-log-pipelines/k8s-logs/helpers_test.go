// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// jobPodStartTimeout is how long we wait for a Job's pod to leave the Pending
// phase. The workload images (ghcr.io/datadog/apps-alpine) are pulled from a
// public registry on a freshly-created kind node, so a cold image pull can take
// well over the previous 30s budget and leave the pod in ContainerCreating.
// Three minutes comfortably absorbs a cold ghcr.io pull without masking real
// failures (bad image names, unschedulable pods, and image-pull backoffs are
// still detected immediately by WaitForJobPodRunning).
const jobPodStartTimeout = 3 * time.Minute

func fakeintakeRouteStats(fi *components.FakeIntake) string {
	stats, err := fi.Client().RouteStats()
	if err != nil {
		return fmt.Sprintf("RouteStats error: %v", err)
	}
	return fmt.Sprintf("RouteStats: %v", stats)
}
