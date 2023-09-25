// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/telemetry"
)

var prometheusDelta deltaCalculator
var prometheusMux sync.Mutex
var prometheusMetrics map[string]any

func ReportPrometheus() {
	prometheusMux.Lock()
	defer prometheusMux.Unlock()

	// Lazily initiate map if necessary
	if prometheusMetrics == nil {
		prometheusMetrics = make(map[string]any)
	}

	deltas := prometheusDelta.GetState("")
	metrics := globalRegistry.GetMetrics(OptPrometheus)
	for _, metric := range metrics {
		base := metric.base()
		pm, ok := prometheusMetrics[base.name]
		if !ok {
			pm = metricToPrometheus(metric)
			prometheusMetrics[base.name] = pm
		}

		switch v := pm.(type) {
		case libtelemetry.Counter:
			deltaValue := deltas.ValueFor(metric)
			v.Add(float64(deltaValue), tagVals(base)...)
		case libtelemetry.Gauge:
			v.Set(float64(base.Get()), tagVals(base)...)
		default:
		}
	}
}

func metricToPrometheus(m metric) any {
	base := m.base()

	// Parse subsystem and name following convention used in the codebase
	//
	// Example: a metric with name `usm.http.hits` will be converted into
	// subsystem: "usm__http"
	// name: "hits"
	subsystem, name := splitName(m)
	subsystem = strings.ReplaceAll(subsystem, ".", "__")

	keys := tagKeys(base)
	if _, ok := m.(*Counter); ok {
		return libtelemetry.NewCounter(subsystem, name, keys, "")
	}

	return libtelemetry.NewGauge(subsystem, name, keys, "")
}

func withTag(m *metricBase, fn func(k, v string)) {
	for _, t := range sets.List(m.tags) {
		pos := strings.IndexByte(t, ':')
		if pos <= 0 {
			continue
		}

		fn(t[:pos], t[pos+1:])
	}
}

func tagKeys(m *metricBase) []string {
	keys := make([]string, 0, len(m.tags))
	withTag(m, func(k, v string) {
		keys = append(keys, k)
	})
	return keys
}

func tagVals(m *metricBase) []string {
	vals := make([]string, 0, len(m.tags))
	withTag(m, func(k, v string) {
		vals = append(vals, v)
	})
	return vals
}
