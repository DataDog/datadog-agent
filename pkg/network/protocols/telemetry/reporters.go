// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// common prefix used across all statsd metric
const statsdPrefix = "datadog.network_tracer."

var statsdDelta deltaCalculator

// ReportStatsd flushes all metrics tagged with `ReportStatsd`
func ReportStatsd() {
	client := getClient()
	if client == nil {
		return
	}

	metrics := GetMetrics(OptStatsd)
	previousValues := statsdDelta.GetState("")
	for _, metric := range metrics {
		v := previousValues.ValueFor(metric)
		if contains(OptGauge, metric.tags) {
			client.Gauge(statsdPrefix+metric.name, float64(v), metric.tags, 1.0) //nolint:errcheck
		}

		client.Count(statsdPrefix+metric.name, v, metric.tags, 1.0) //nolint:errcheck
	}
}

var telemetryDelta deltaCalculator

// ReportPayloadTelemetry returns a map with all metrics tagged with `OptTelemetry`
// The return format is consistent with what we use in the protobuf messages sent to the backend
func ReportPayloadTelemetry(clientID string) map[string]int64 {
	metrics := GetMetrics(OptTelemetry)
	previousValues := telemetryDelta.GetState(clientID)
	result := make(map[string]int64, len(metrics))
	for _, metric := range metrics {
		result[metric.name] = previousValues.ValueFor(metric)
	}
	return result
}

var expvarDelta deltaCalculator

// ReportExpvar returns a nested map structure with all metrics tagged with `OptExpvar`
func ReportExpvar() map[string]interface{} {
	metrics := GetMetrics(OptExpvar)
	previousValues := expvarDelta.GetState("")
	root := make(map[string]interface{})
	seen := make(map[string]struct{})

	for _, m := range metrics {
		if _, ok := seen[m.name]; ok {
			log.Debugf(
				"metric %q has multiple instances with different tag sets which is not suitable for expvar.",
				m.name,
			)
			continue
		}

		seen[m.name] = struct{}{}
		err := insertNestedValueFor(m.name, previousValues.ValueFor(m), root)
		if err != nil {
			log.Errorf("error inserting metric %s into expvar map: %s", m.name, err)
		}
	}

	return root
}

var clientMux sync.Mutex
var client statsd.ClientInterface

// SetStatsdClient used to report data during invocations of `ReportStatsd`
// TODO: should `ReportStatsd` receive a client instance instead?
func SetStatsdClient(c statsd.ClientInterface) {
	clientMux.Lock()
	defer clientMux.Unlock()
	client = c
}

func getClient() statsd.ClientInterface {
	clientMux.Lock()
	defer clientMux.Unlock()
	return client
}
