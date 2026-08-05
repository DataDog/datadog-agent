// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package numamonitoring

import (
	"math"
	"time"
)

func distribution(values map[int]uint64) map[int]float64 {
	var total uint64
	for _, value := range values {
		total += value
	}
	if total == 0 {
		return nil
	}

	result := make(map[int]float64, len(values))
	for node, value := range values {
		result[node] = float64(value) / float64(total)
	}
	return result
}

func placementMismatch(runtime, residency map[int]float64) (float64, bool) {
	if len(runtime) == 0 || len(residency) == 0 {
		return 0, false
	}

	nodes := make(map[int]struct{}, len(runtime)+len(residency))
	for node := range runtime {
		nodes[node] = struct{}{}
	}
	for node := range residency {
		nodes[node] = struct{}{}
	}

	var distance float64
	for node := range nodes {
		distance += math.Abs(runtime[node] - residency[node])
	}
	return 0.5 * distance, true
}

func counterRate(previous, current uint64, elapsed time.Duration) (float64, bool) {
	if elapsed <= 0 || current < previous {
		return 0, false
	}
	return float64(current-previous) / elapsed.Seconds(), true
}

func remoteRatio(total, local float64) (remote float64, ratio float64, ok bool) {
	if total <= 0 || local < 0 {
		return 0, 0, false
	}
	remote = math.Max(0, total-local)
	return remote, remote / total, true
}

func badnessScore(mismatch float64, remote *float64) float64 {
	if remote != nil && *remote > mismatch {
		return *remote
	}
	return mismatch
}
