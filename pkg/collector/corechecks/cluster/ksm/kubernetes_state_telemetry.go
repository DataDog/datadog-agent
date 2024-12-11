// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

type telemetryCache struct {
	totalCount             int
	unknownMetricsCount    int
	metricsCountByResource map[string]int
}

func newTelemetryCache() *telemetryCache {
	return &telemetryCache{
		totalCount:             0,
		unknownMetricsCount:    0,
		metricsCountByResource: make(map[string]int),
	}
}

func (t *telemetryCache) reset() {
	t.totalCount = 0
	t.unknownMetricsCount = 0
	t.metricsCountByResource = make(map[string]int)
}

func (t *telemetryCache) incTotal(val int) {
	t.totalCount += val
}

func (t *telemetryCache) getTotal() int {
	return t.totalCount
}

func (t *telemetryCache) incUnknown() {
	t.unknownMetricsCount++
}

func (t *telemetryCache) getUnknown() int {
	return t.unknownMetricsCount
}

func (t *telemetryCache) incResource(resource string, val int) {
	t.metricsCountByResource[resource] += val
}

func (t *telemetryCache) getResourcesCount() map[string]int {
	return t.metricsCountByResource
}
