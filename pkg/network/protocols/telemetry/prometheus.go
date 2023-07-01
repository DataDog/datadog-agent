// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"strings"
	"sync"

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
	metrics := GetMetrics(OptPrometheus)
	for _, m := range metrics {
		name := m.name
		pm, ok := prometheusMetrics[name]
		if !ok {
			pm = metricToPrometheus(m)
			prometheusMetrics[name] = pm
		}

		switch v := pm.(type) {
		case libtelemetry.Counter:
			deltaValue := deltas.ValueFor(m)
			v.Add(float64(deltaValue), tagVals(m)...)
		case libtelemetry.Gauge:
			v.Set(float64(m.Get()), tagVals(m)...)
		default:
		}
	}
}

func metricToPrometheus(m *Metric) any {
	subsystem := ""
	name := m.name

	// Parse subsystem and name following convention used in the codebase
	//
	// Example: a metric with name `usm.http.hits` will be converted into
	// subsystem: "usm__http"
	// name: "hits"
	separatorPos := strings.LastIndex(name, ".")
	if separatorPos > 0 && separatorPos < len(name)-1 {
		subsystem = name[:separatorPos]
		name = name[separatorPos+1:]
		subsystem = strings.ReplaceAll(subsystem, ".", "__")
	}

	keys := tagKeys(m)
	if m.metricType == typeCounter {
		return libtelemetry.NewCounter(subsystem, name, keys, "")
	}

	return libtelemetry.NewGauge(subsystem, name, keys, "")
}

func withTag(m *Metric, fn func(k, v string)) {
	for _, t := range m.tags.List() {
		pos := strings.IndexByte(t, ':')
		if pos <= 0 {
			continue
		}

		fn(t[:pos], t[pos+1:])
	}
}

func tagKeys(m *Metric) []string {
	keys := make([]string, 0, len(m.tags))
	withTag(m, func(k, v string) {
		keys = append(keys, k)
	})
	return keys
}

func tagVals(m *Metric) []string {
	vals := make([]string, 0, len(m.tags))
	withTag(m, func(k, v string) {
		vals = append(vals, v)
	})
	return vals
}
