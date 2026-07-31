// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"fmt"
	"slices"
	"sync"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

// This file holds the Agent Telemetry registry backing the datadog_agent.emit_agent_telemetry
// CGO callback (EmitAgentTelemetry in datadog_agent.go). Python supplies the metric name, type,
// and label names at runtime, so each metric is declared on first use and cached here.
//
// The cached type and label names are load bearing. Registering the same metric twice panics in
// prometheus.Registry.MustRegister, and updating a metric with a label set it was not declared
// with panics in prometheus's With, so both conflicts must be detected here and reported as
// errors: the caller is a CGO callback with no error channel, and a panic would take the Agent
// down on data supplied by a check.

var (
	telemetryMap  = map[string]*agentTelemetryMetric{}
	telemetryLock = sync.Mutex{}
)

type agentTelemetryMetric struct {
	metric     any
	metricType string
	labelNames []string
}

func lazyInitTelemetryMetric(checkName string, metricName string, metricType string, labelNames []string) (*agentTelemetryMetric, error) {
	key := checkName + "." + metricName
	telemetryLock.Lock()
	defer telemetryLock.Unlock()

	if entry, ok := telemetryMap[key]; ok {
		if entry.metricType != metricType {
			return nil, fmt.Errorf("metric %s for check %s was already emitted as %s when %s was expected", metricName, checkName, entry.metricType, metricType)
		}
		if !slices.Equal(entry.labelNames, labelNames) {
			return nil, fmt.Errorf("metric %s for check %s was already emitted with labels %v when labels %v were expected", metricName, checkName, entry.labelNames, labelNames)
		}
		return entry, nil
	}

	entry := &agentTelemetryMetric{
		metricType: metricType,
		labelNames: slices.Clone(labelNames),
	}
	switch metricType {
	case "counter":
		entry.metric = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
			checkName,
			metricName,
			entry.labelNames,
			fmt.Sprintf("Counter of %s for Python check %s", metricName, checkName),
			telemetry.DefaultOptions,
		)
	case "histogram":
		entry.metric = telemetryimpl.GetCompatComponent().NewHistogramWithOpts(
			checkName,
			metricName,
			entry.labelNames,
			fmt.Sprintf("Histogram of %s for Python check %s", metricName, checkName),
			[]float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000},
			telemetry.DefaultOptions,
		)
	case "gauge":
		entry.metric = telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
			checkName,
			metricName,
			entry.labelNames,
			fmt.Sprintf("Gauge of %s for Python check %s", metricName, checkName),
			telemetry.DefaultOptions,
		)
	default:
		return nil, fmt.Errorf("unsupported metric type %s requested by %s for %s", metricType, checkName, metricName)
	}

	telemetryMap[key] = entry
	return entry, nil
}
