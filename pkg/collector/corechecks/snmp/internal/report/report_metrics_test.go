// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func TestSendMetric(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		caseName           string
		symbol             checkconfig.SymbolConfig
		value              valuestore.ResultValue
		tags               []string
		metricConfig       checkconfig.MetricsConfig
		expectedMethod     string
		expectedMetricName string
		expectedValue      float64
		expectedTags       []string
		expectedSubMetrics int
		expectedLogs       []logCount
	}{
		{
			caseName:           "Gauge metric case",
			symbol:             checkconfig.SymbolConfig{Name: "gauge.metric"},
			value:              valuestore.ResultValue{SubmissionType: "gauge", Value: float64(10)},
			tags:               []string{},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.gauge.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Counter32 metric case",
			symbol:             checkconfig.SymbolConfig{Name: "counter.metric"},
			value:              valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:               []string{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.counter.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName: "Forced gauge metric case",
			symbol:   checkconfig.SymbolConfig{Name: "my.metric"},
			value:    valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "gauge",
			},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName: "Forced counter metric case",
			symbol:   checkconfig.SymbolConfig{Name: "my.metric"},
			value:    valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "counter",
			},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName: "Forced monotonic_count metric case",
			symbol:   checkconfig.SymbolConfig{Name: "my.metric"},
			value:    valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "monotonic_count",
			},
			expectedMethod:     "MonotonicCount",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName: "Forced monotonic_count_and_rate metric case: MonotonicCount called",
			symbol:   checkconfig.SymbolConfig{Name: "my.metric"},
			value:    valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "monotonic_count_and_rate",
			},
			expectedMethod:     "MonotonicCount",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 2,
		},
		{
			caseName: "Forced monotonic_count_and_rate metric case: Rate called",
			symbol:   checkconfig.SymbolConfig{Name: "my.metric"},
			value:    valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "monotonic_count_and_rate",
			},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.my.metric.rate",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 2,
		},
		{
			caseName: "Forced percent metric case: Rate called",
			symbol:   checkconfig.SymbolConfig{Name: "Rate.metric"},
			value:    valuestore.ResultValue{Value: 0.5},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "percent",
			},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.Rate.metric",
			expectedValue:      50.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName: "Forced flag_stream case 1",
			symbol:   checkconfig.SymbolConfig{Name: "metric"},
			value:    valuestore.ResultValue{Value: "1010"},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "flag_stream",
				Options:    checkconfig.MetricsConfigOption{Placement: 1, MetricSuffix: "foo"},
			},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.metric.foo",
			expectedValue:      1.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName: "Forced flag_stream case 2",
			symbol:   checkconfig.SymbolConfig{Name: "metric"},
			value:    valuestore.ResultValue{Value: "1010"},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "flag_stream",
				Options:    checkconfig.MetricsConfigOption{Placement: 2, MetricSuffix: "bar"},
			},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.metric.bar",
			expectedValue:      0.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName: "Forced flag_stream invalid index",
			symbol:   checkconfig.SymbolConfig{Name: "metric"},
			value:    valuestore.ResultValue{Value: "1010"},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "flag_stream",
				Options:    checkconfig.MetricsConfigOption{Placement: 10, MetricSuffix: "none"},
			},
			expectedMethod:     "",
			expectedMetricName: "",
			expectedValue:      0.0,
			expectedTags:       []string{},
			expectedSubMetrics: 0,
			expectedLogs: []logCount{
				{"[DEBUG] sendMetric: metric `snmp.metric`: failed to get flag stream value: flag stream index `9` not found in `1010`", 1},
			},
		},
		{
			caseName: "Error converting value",
			symbol:   checkconfig.SymbolConfig{Name: "metric"},
			value:    valuestore.ResultValue{Value: valuestore.ResultValue{}},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "flag_stream",
				Options:    checkconfig.MetricsConfigOption{Placement: 10, MetricSuffix: "ouch"},
			},
			expectedMethod:     "",
			expectedMetricName: "",
			expectedValue:      0.0,
			expectedTags:       []string{},
			expectedSubMetrics: 0,
			expectedLogs: []logCount{
				{"[DEBUG] sendMetric: error converting value", 1},
			},
		},
		{
			caseName:           "Cannot convert value to float",
			symbol:             checkconfig.SymbolConfig{Name: "gauge.metric"},
			value:              valuestore.ResultValue{Value: "abc"},
			tags:               []string{},
			expectedMethod:     "",
			expectedMetricName: "",
			expectedValue:      0,
			expectedTags:       []string{},
			expectedSubMetrics: 0,
			expectedLogs: []logCount{
				{"[DEBUG] sendMetric: metric `snmp.gauge.metric`: failed to convert to float64", 1},
			},
		},
		{
			caseName: "Unsupported type",
			symbol:   checkconfig.SymbolConfig{Name: "gauge.metric"},
			value:    valuestore.ResultValue{Value: "1"},
			tags:     []string{},
			metricConfig: checkconfig.MetricsConfig{
				ForcedType: "invalidForceType",
			},
			expectedMethod:     "",
			expectedMetricName: "",
			expectedValue:      0,
			expectedTags:       []string{},
			expectedSubMetrics: 0,
			expectedLogs: []logCount{
				{"[DEBUG] sendMetric: metric `snmp.gauge.metric`: unsupported forcedType: invalidForceType", 1},
			},
		},
		{
			caseName: "Scaled value",
			symbol: checkconfig.SymbolConfig{
				Name:        "scaled.metric",
				ScaleFactor: 2,
			},
			value:              valuestore.ResultValue{SubmissionType: "gauge", Value: float64(10)},
			tags:               []string{},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.scaled.metric",
			expectedValue:      float64(20),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Scaled value by float",
			symbol:             checkconfig.SymbolConfig{Name: "scaled.metric", ScaleFactor: 0.5},
			value:              valuestore.ResultValue{SubmissionType: "gauge", Value: float64(10)},
			tags:               []string{},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.scaled.metric",
			expectedValue:      float64(5),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.caseName, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)

			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			log.SetupLogger(l, "debug")

			mockSender := mocksender.NewMockSender("foo")
			metricSender := MetricSender{sender: mockSender}
			mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sample := MetricSample{
				value:      tt.value,
				tags:       tt.tags,
				symbol:     tt.symbol,
				forcedType: tt.metricConfig.ForcedType,
				options:    tt.metricConfig.Options,
			}
			metricSender.sendMetric(sample)
			assert.Equal(t, tt.expectedSubMetrics, metricSender.submittedMetrics)
			if tt.expectedMethod != "" {
				mockSender.AssertCalled(t, tt.expectedMethod, tt.expectedMetricName, tt.expectedValue, "", tt.expectedTags)
			}

			w.Flush()
			logs := b.String()

			for _, aLogCount := range tt.expectedLogs {
				assert.Equal(t, aLogCount.count, strings.Count(logs, aLogCount.log), logs)
			}
		})
	}
}

func Test_metricSender_reportMetrics(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		name         string
		metrics      []checkconfig.MetricsConfig
		values       *valuestore.ResultValueStore
		tags         []string
		expectedLogs []logCount
	}{
		{
			name: "report scalar error",
			metrics: []checkconfig.MetricsConfig{
				{Symbol: checkconfig.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			values: &valuestore.ResultValueStore{},
			expectedLogs: []logCount{
				{"[DEBUG] reportScalarMetrics: report scalar: error getting scalar value: value for Scalar OID `1.2.3.4.5` not found in results", 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)

			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			log.SetupLogger(l, "debug")

			mockSender := mocksender.NewMockSender("foo")
			mockSender.SetupAcceptAll()

			metricSender := MetricSender{sender: mockSender}

			metricSender.ReportMetrics(tt.metrics, tt.values, tt.tags)

			w.Flush()
			logs := b.String()

			for _, aLogCount := range tt.expectedLogs {
				assert.Equal(t, aLogCount.count, strings.Count(logs, aLogCount.log), logs)
			}
		})
	}
}

func Test_metricSender_getCheckInstanceMetricTags(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		name         string
		metricsTags  []checkconfig.MetricTagConfig
		values       *valuestore.ResultValueStore
		expectedTags []string
		expectedLogs []logCount
	}{
		{
			name: "no scalar oids found",
			metricsTags: []checkconfig.MetricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
				{Tag: "snmp_host", OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
			},
			values:       &valuestore.ResultValueStore{},
			expectedTags: []string{},
			expectedLogs: []logCount{},
		},
		{
			name: "report scalar tags with regex",
			metricsTags: []checkconfig.MetricTagConfig{
				{OID: "1.2.3", Name: "mySymbol", Match: "^([a-zA-Z]+)([0-9]+)$", Tags: map[string]string{
					"word":   "\\1",
					"number": "\\2",
				}},
			},
			values: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.2.3": valuestore.ResultValue{
						Value: "hello123",
					},
				},
			},
			expectedTags: []string{"word:hello", "number:123"},
			expectedLogs: []logCount{},
		},
		{
			name: "error converting tag value",
			metricsTags: []checkconfig.MetricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
			},
			values: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.2.3": valuestore.ResultValue{
						Value: valuestore.ResultValue{},
					},
				},
			},
			expectedLogs: []logCount{
				{"error converting value", 1},
			},
		},
		{
			name: "tag value mapping",
			metricsTags: []checkconfig.MetricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol", Mapping: map[string]string{"1": "one", "2": "two"}},
			},
			values: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.2.3": valuestore.ResultValue{
						Value: float64(2),
					},
				},
			},
			expectedTags: []string{"my_symbol:two"},
			expectedLogs: []logCount{},
		},
		{
			name: "invalid tag value mapping",
			metricsTags: []checkconfig.MetricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol", Mapping: map[string]string{"1": "one", "2": "two"}},
			},
			values: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.2.3": valuestore.ResultValue{
						Value: float64(3),
					},
				},
			},
			expectedTags: []string(nil),
			expectedLogs: []logCount{{"error getting tags", 1}},
		},
		{
			name: "empty tag value mapping",
			metricsTags: []checkconfig.MetricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol", Mapping: map[string]string{}},
			},
			values: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.2.3": valuestore.ResultValue{
						Value: float64(3),
					},
				},
			},
			expectedTags: []string{"my_symbol:3"},
			expectedLogs: []logCount{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)

			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			log.SetupLogger(l, "debug")

			mockSender := mocksender.NewMockSender("foo")
			metricSender := MetricSender{sender: mockSender}

			checkconfig.ValidateEnrichMetricTags(tt.metricsTags)
			tags := metricSender.GetCheckInstanceMetricTags(tt.metricsTags, tt.values)

			assert.ElementsMatch(t, tt.expectedTags, tags)

			w.Flush()
			logs := b.String()

			for _, aLogCount := range tt.expectedLogs {
				assert.Equal(t, strings.Count(logs, aLogCount.log), aLogCount.count, logs)
			}
		})
	}
}
