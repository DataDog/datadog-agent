// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
	"k8s.io/apimachinery/pkg/util/sets"
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

	metrics := globalRegistry.GetMetrics(OptStatsd)
	previousValues := statsdDelta.GetState("")
	for _, metric := range metrics {
		v := previousValues.ValueFor(metric)
		base := metric.base()
		tags := sets.List(base.tags)
		if _, ok := metric.(*Gauge); ok {
			client.Gauge(statsdPrefix+base.name, float64(v), tags, 1.0) //nolint:errcheck
			continue
		}

		client.Count(statsdPrefix+base.name, v, tags, 1.0) //nolint:errcheck
	}
}

var telemetryDelta deltaCalculator

// ReportPayloadTelemetry returns a map with all metrics tagged with `OptPayloadTelemetry`
// The return format is consistent with what we use in the protobuf messages sent to the backend
func ReportPayloadTelemetry(clientID string) map[string]int64 {
	metrics := globalRegistry.GetMetrics(OptPayloadTelemetry)
	previousValues := telemetryDelta.GetState(clientID)
	result := make(map[string]int64, len(metrics))
	for _, metric := range metrics {
		result[metric.base().name] = previousValues.ValueFor(metric)
	}
	return result
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
