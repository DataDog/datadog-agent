package snmp

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestSendMetric(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		caseName            string
		metricName          string
		value               snmpValueType
		tags                []string
		forcedType          string
		options             metricsConfigOption
		extractValuePattern *regexp.Regexp
		expectedMethod      string
		expectedMetricName  string
		expectedValue       float64
		expectedTags        []string
		expectedSubMetrics  int
		expectedLogs        []logCount
	}{
		{
			caseName:           "Gauge metric case",
			metricName:         "gauge.metric",
			value:              snmpValueType{submissionType: "gauge", value: float64(10)},
			tags:               []string{},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.gauge.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Counter32 metric case",
			metricName:         "counter.metric",
			value:              snmpValueType{submissionType: "counter", value: float64(10)},
			tags:               []string{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.counter.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced gauge metric case",
			metricName:         "my.metric",
			value:              snmpValueType{submissionType: "counter", value: float64(10)},
			tags:               []string{},
			forcedType:         "gauge",
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced counter metric case",
			metricName:         "my.metric",
			value:              snmpValueType{submissionType: "counter", value: float64(10)},
			tags:               []string{},
			forcedType:         "counter",
			options:            metricsConfigOption{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced monotonic_count metric case",
			metricName:         "my.metric",
			value:              snmpValueType{submissionType: "counter", value: float64(10)},
			tags:               []string{},
			forcedType:         "monotonic_count",
			options:            metricsConfigOption{},
			expectedMethod:     "MonotonicCount",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced monotonic_count_and_rate metric case: MonotonicCount called",
			metricName:         "my.metric",
			value:              snmpValueType{submissionType: "counter", value: float64(10)},
			tags:               []string{},
			forcedType:         "monotonic_count_and_rate",
			options:            metricsConfigOption{},
			expectedMethod:     "MonotonicCount",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 2,
		},
		{
			caseName:           "Forced monotonic_count_and_rate metric case: Rate called",
			metricName:         "my.metric",
			value:              snmpValueType{submissionType: "counter", value: float64(10)},
			tags:               []string{},
			forcedType:         "monotonic_count_and_rate",
			options:            metricsConfigOption{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.my.metric.rate",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 2,
		},
		{
			caseName:           "Forced percent metric case: Rate called",
			metricName:         "rate.metric",
			value:              snmpValueType{value: 0.5},
			tags:               []string{},
			forcedType:         "percent",
			options:            metricsConfigOption{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.rate.metric",
			expectedValue:      50.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced flag_stream case 1",
			metricName:         "metric",
			value:              snmpValueType{value: "1010"},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            metricsConfigOption{Placement: 1, MetricSuffix: "foo"},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.metric.foo",
			expectedValue:      1.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced flag_stream case 2",
			metricName:         "metric",
			value:              snmpValueType{value: "1010"},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            metricsConfigOption{Placement: 2, MetricSuffix: "foo"},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.metric.foo",
			expectedValue:      0.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced flag_stream invalid index",
			metricName:         "metric",
			value:              snmpValueType{value: "1010"},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            metricsConfigOption{Placement: 10, MetricSuffix: "foo"},
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
			caseName:           "Error converting value",
			metricName:         "metric",
			value:              snmpValueType{value: snmpValueType{}},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            metricsConfigOption{Placement: 10, MetricSuffix: "foo"},
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
			metricName:         "gauge.metric",
			value:              snmpValueType{value: "abc"},
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
			caseName:           "Unsupported type",
			metricName:         "gauge.metric",
			value:              snmpValueType{value: "1"},
			tags:               []string{},
			forcedType:         "invalidForceType",
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
			caseName:            "Extract Value OK case",
			metricName:          "gauge.metric",
			value:               snmpValueType{submissionType: "gauge", value: string("22C")},
			tags:                []string{},
			extractValuePattern: regexp.MustCompile(`(\d+)C`),
			expectedMethod:      "Gauge",
			expectedMetricName:  "snmp.gauge.metric",
			expectedValue:       float64(22),
			expectedTags:        []string{},
			expectedSubMetrics:  1,
		},
		{
			caseName:            "Extract Value not matched",
			metricName:          "gauge.metric",
			value:               snmpValueType{submissionType: "gauge", value: string("NOMATCH")},
			tags:                []string{},
			extractValuePattern: regexp.MustCompile(`(\d+)C`),
			expectedMethod:      "",
			expectedMetricName:  "",
			expectedValue:       0,
			expectedTags:        []string{},
			expectedSubMetrics:  0,
			expectedLogs: []logCount{
				{"[DEBUG] sendMetric: error extracting value from", 1},
			},
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
			metricSender := metricSender{sender: mockSender}
			mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			metricSender.sendMetric(tt.metricName, tt.value, tt.tags, tt.forcedType, tt.options, tt.extractValuePattern)
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
		metrics      []metricsConfig
		values       *resultValueStore
		tags         []string
		expectedLogs []logCount
	}{
		{
			name: "report scalar error",
			metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			values: &resultValueStore{},
			expectedLogs: []logCount{
				{"[DEBUG] reportScalarMetrics: report scalar: error getting scalar value: value for Scalar OID `1.2.3.4.5` not found in results", 1},
			},
		},
		{
			name: "report column error",
			metrics: []metricsConfig{
				{
					ForcedType: "monotonic_count",
					Symbols: []symbolConfig{
						{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
						{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
					},
					MetricTags: []metricTagConfig{
						{Tag: "interface", Column: symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
						{Tag: "interface_alias", Column: symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
					},
				},
			},
			values: &resultValueStore{},
			expectedLogs: []logCount{
				{"[DEBUG] reportColumnMetrics: report column: error getting column value: value for Column OID `1.3.6.1.2.1.2.2.1.13` not found in results", 1},
				{"[DEBUG] reportColumnMetrics: report column: error getting column value: value for Column OID `1.3.6.1.2.1.2.2.1.14` not found in results", 1},
			},
		},
		{
			name: "report column cache",
			metrics: []metricsConfig{
				{
					ForcedType: "monotonic_count",
					Symbols: []symbolConfig{
						{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
						{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
					},
					MetricTags: []metricTagConfig{
						{Tag: "interface", Column: symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
					},
				},
			},
			values: &resultValueStore{
				columnValues: columnResultValuesType{
					"1.3.6.1.2.1.2.2.1.14": map[string]snmpValueType{
						"10": {
							"gauge",
							10,
						},
					},
					"1.3.6.1.2.1.2.2.1.13": map[string]snmpValueType{
						"10": {
							"gauge",
							10,
						},
					},
					"1.3.6.1.2.1.31.1.1.1.1": map[string]snmpValueType{
						"10": {
							value: "myIfName",
						},
					},
				},
			},
			expectedLogs: []logCount{
				{"[DEBUG] reportColumnMetrics: report column: caching tags `[interface:myIfName]` for fullIndex `10`", 1},
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

			metricSender := metricSender{sender: mockSender}

			metricSender.reportMetrics(tt.metrics, tt.values, tt.tags)

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
		metricsTags  []metricTagConfig
		values       *resultValueStore
		expectedTags []string
		expectedLogs []logCount
	}{
		{
			name: "report scalar error",
			metricsTags: []metricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
				{Tag: "snmp_host", OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
			},
			values: &resultValueStore{},
			expectedLogs: []logCount{
				{"[DEBUG] getCheckInstanceMetricTags: metric tags: error getting scalar value: value for Scalar OID `1.2.3` not found in results", 1},
				{"[DEBUG] getCheckInstanceMetricTags: metric tags: error getting scalar value: value for Scalar OID `1.3.6.1.2.1.1.5.0` not found in results", 1},
			},
		},
		{
			name: "report scalar tags with regex",
			metricsTags: []metricTagConfig{
				{OID: "1.2.3", Name: "mySymbol", Match: "^([a-zA-Z]+)([0-9]+)$", Tags: map[string]string{
					"word":   "\\1",
					"number": "\\2",
				}},
			},
			values: &resultValueStore{
				scalarValues: scalarResultValuesType{
					"1.2.3": snmpValueType{
						value: "hello123",
					},
				},
			},
			expectedTags: []string{"word:hello", "number:123"},
			expectedLogs: []logCount{},
		},
		{
			name: "error converting tag value",
			metricsTags: []metricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
			},
			values: &resultValueStore{
				scalarValues: scalarResultValuesType{
					"1.2.3": snmpValueType{
						value: snmpValueType{},
					},
				},
			},
			expectedLogs: []logCount{
				{"error converting value", 1},
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
			metricSender := metricSender{sender: mockSender}

			validateEnrichMetricTags(tt.metricsTags)
			tags := metricSender.getCheckInstanceMetricTags(tt.metricsTags, tt.values)

			assert.ElementsMatch(t, tt.expectedTags, tags)

			w.Flush()
			logs := b.String()

			for _, aLogCount := range tt.expectedLogs {
				assert.Equal(t, strings.Count(logs, aLogCount.log), aLogCount.count, logs)
			}
		})
	}
}
