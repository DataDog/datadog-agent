// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func Test_metricSender_sendMemoryUsageMetric(t *testing.T) {
	type MetricSamplesStore struct {
		scalarSamples map[string]MetricSample
		columnSamples map[string]map[string]MetricSample
	}
	type Metric struct {
		name  string
		value float64
		tags  []string
	}
	tests := []struct {
		name            string
		samplesStore    MetricSamplesStore
		expectedMetrics []Metric
		expectedError   error
	}{
		{
			"should not emit evaluated snmp.memory.usage when scalar memory.usage is collected",
			MetricSamplesStore{scalarSamples: map[string]MetricSample{
				"memory.usage": {
					value:      valuestore.ResultValue{Value: 100.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
			}},
			[]Metric{},
			nil,
		},
		{
			"should not emit evaluated snmp.memory.usage when column memory.usage is collected",
			MetricSamplesStore{columnSamples: map[string]map[string]MetricSample{
				"memory.usage": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
			}},
			[]Metric{},
			nil,
		},
		{
			"should not emit evaluated snmp.memory.usage when only scalar memory.used is collected",
			MetricSamplesStore{scalarSamples: map[string]MetricSample{
				"memory.used": {
					value:      valuestore.ResultValue{Value: 100.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.used"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
			}},
			[]Metric{},
			fmt.Errorf("missing free, total memory metrics, skipping scalar memory usage"),
		},
		{
			"should not emit evaluated snmp.memory.usage when only scalar memory.free is collected",
			MetricSamplesStore{scalarSamples: map[string]MetricSample{
				"memory.free": {
					value:      valuestore.ResultValue{Value: 100.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.free"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
			}},
			[]Metric{},
			fmt.Errorf("missing used, total memory metrics, skipping scalar memory usage"),
		},
		{
			"should not emit evaluated snmp.memory.usage when only scalar memory.total is collected",
			MetricSamplesStore{scalarSamples: map[string]MetricSample{
				"memory.total": {
					value:      valuestore.ResultValue{Value: 100.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.total"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
			}},
			[]Metric{},
			fmt.Errorf("missing used, free memory metrics, skipping scalar memory usage"),
		},
		{
			"should not emit evaluated snmp.memory.usage when only column memory.used is collected",
			MetricSamplesStore{columnSamples: map[string]map[string]MetricSample{
				"memory.used": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.used"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.used"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
			}},
			[]Metric{},
			fmt.Errorf("missing free, total memory metrics, skipping column memory usage"),
		},
		{
			"should not emit evaluated snmp.memory.usage when only column memory.free is collected",
			MetricSamplesStore{columnSamples: map[string]map[string]MetricSample{
				"memory.free": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.free"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.free"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
			}},
			[]Metric{},
			fmt.Errorf("missing used, total memory metrics, skipping column memory usage"),
		},
		{
			"should not emit evaluated snmp.memory.usage when only column memory.total is collected",
			MetricSamplesStore{columnSamples: map[string]map[string]MetricSample{
				"memory.total": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.total"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.total"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
			}},
			[]Metric{},
			fmt.Errorf("missing used, free memory metrics, skipping column memory usage"),
		},
		{
			"should not emit evaluated snmp.memory.usage when no memory metric is collected",
			MetricSamplesStore{},
			[]Metric{},
			fmt.Errorf("missing used, free, total memory metrics, skipping column memory usage"),
		},
		{
			"should emit evaluated snmp.memory.usage when scalar memory.used and memory.total are collected",
			MetricSamplesStore{scalarSamples: map[string]MetricSample{
				"memory.used": {
					value:      valuestore.ResultValue{Value: 50.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.used"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
				"memory.total": {
					value:      valuestore.ResultValue{Value: 200.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.total"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
			}},
			[]Metric{
				{"snmp.memory.usage", 25.0, []string{"device_namespace:default", "ip_address:192.168.10.24"}},
			},
			nil,
		},
		{
			"should emit evaluated snmp.memory.usage when scalar memory.used and memory.free are collected",
			MetricSamplesStore{scalarSamples: map[string]MetricSample{
				"memory.used": {
					value:      valuestore.ResultValue{Value: 50.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.used"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
				"memory.free": {
					value:      valuestore.ResultValue{Value: 150.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.free"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
			}},
			[]Metric{
				{"snmp.memory.usage", 25.0, []string{"device_namespace:default", "ip_address:192.168.10.24"}},
			},
			nil,
		},
		{
			"should emit evaluated snmp.memory.usage when scalar memory.free and memory.total are collected",
			MetricSamplesStore{scalarSamples: map[string]MetricSample{
				"memory.free": {
					value:      valuestore.ResultValue{Value: 150.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.free"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
				"memory.total": {
					value:      valuestore.ResultValue{Value: 200.0},
					tags:       []string{"device_namespace:default", "ip_address:192.168.10.24"},
					symbol:     profiledefinition.SymbolConfig{Name: "memory.total"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				},
			}},
			[]Metric{
				{"snmp.memory.usage", 25.0, []string{"device_namespace:default", "ip_address:192.168.10.24"}},
			},
			nil,
		},
		{
			"should emit evaluated snmp.memory.usage when column memory.used and memory.total are collected",
			MetricSamplesStore{columnSamples: map[string]map[string]MetricSample{
				"memory.used": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.used"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 20.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.used"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
				"memory.total": {
					"123": {
						value:      valuestore.ResultValue{Value: 200.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.total"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 200.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.total"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
			}},
			[]Metric{
				{"snmp.memory.usage", 50.0, []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"}},
				{"snmp.memory.usage", 10.0, []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"}},
			},
			nil,
		},
		{
			"should emit evaluated snmp.memory.usage when column memory.used and memory.free are collected",
			MetricSamplesStore{columnSamples: map[string]map[string]MetricSample{
				"memory.used": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 20.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
				"memory.free": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 180.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
			}},
			[]Metric{
				{"snmp.memory.usage", 50.0, []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"}},
				{"snmp.memory.usage", 10.0, []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"}},
			},
			nil,
		},
		{
			"should emit evaluated snmp.memory.usage when column memory.free and memory.total are collected",
			MetricSamplesStore{columnSamples: map[string]map[string]MetricSample{
				"memory.free": {
					"123": {
						value:      valuestore.ResultValue{Value: 100.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 180.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
				"memory.total": {
					"123": {
						value:      valuestore.ResultValue{Value: 200.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
					"567": {
						value:      valuestore.ResultValue{Value: 200.0},
						tags:       []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"},
						symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
						options:    profiledefinition.MetricsConfigOption{},
						forcedType: "",
					},
				},
			}},
			[]Metric{
				{"snmp.memory.usage", 50.0, []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:123"}},
				{"snmp.memory.usage", 10.0, []string{"device_namespace:default", "ip_address:192.168.10.24", "mem:567"}},
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("testID") // required to initiate aggregator
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			ms := &MetricSender{
				sender: sender,
			}
			err := ms.tryReportMemoryUsage(tt.samplesStore.scalarSamples, tt.samplesStore.columnSamples)
			assert.Equal(t, tt.expectedError, err)

			sender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))

			for _, metric := range tt.expectedMetrics {
				sender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}
		})
	}
}
