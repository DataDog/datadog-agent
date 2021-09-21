package report

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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
)

func TestSendMetric(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		caseName            string
		metricName          string
		value               valuestore.ResultValue
		tags                []string
		forcedType          string
		options             checkconfig.MetricsConfigOption
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
			metricName:         "counter.metric",
			value:              valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
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
			value:              valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
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
			value:              valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:               []string{},
			forcedType:         "counter",
			options:            checkconfig.MetricsConfigOption{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced monotonic_count metric case",
			metricName:         "my.metric",
			value:              valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:               []string{},
			forcedType:         "monotonic_count",
			options:            checkconfig.MetricsConfigOption{},
			expectedMethod:     "MonotonicCount",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced monotonic_count_and_rate metric case: MonotonicCount called",
			metricName:         "my.metric",
			value:              valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:               []string{},
			forcedType:         "monotonic_count_and_rate",
			options:            checkconfig.MetricsConfigOption{},
			expectedMethod:     "MonotonicCount",
			expectedMetricName: "snmp.my.metric",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 2,
		},
		{
			caseName:           "Forced monotonic_count_and_rate metric case: Rate called",
			metricName:         "my.metric",
			value:              valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			tags:               []string{},
			forcedType:         "monotonic_count_and_rate",
			options:            checkconfig.MetricsConfigOption{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.my.metric.rate",
			expectedValue:      float64(10),
			expectedTags:       []string{},
			expectedSubMetrics: 2,
		},
		{
			caseName:           "Forced percent metric case: Rate called",
			metricName:         "Rate.metric",
			value:              valuestore.ResultValue{Value: 0.5},
			tags:               []string{},
			forcedType:         "percent",
			options:            checkconfig.MetricsConfigOption{},
			expectedMethod:     "Rate",
			expectedMetricName: "snmp.Rate.metric",
			expectedValue:      50.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced flag_stream case 1",
			metricName:         "metric",
			value:              valuestore.ResultValue{Value: "1010"},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            checkconfig.MetricsConfigOption{Placement: 1, MetricSuffix: "foo"},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.metric.foo",
			expectedValue:      1.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced flag_stream case 2",
			metricName:         "metric",
			value:              valuestore.ResultValue{Value: "1010"},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            checkconfig.MetricsConfigOption{Placement: 2, MetricSuffix: "foo"},
			expectedMethod:     "Gauge",
			expectedMetricName: "snmp.metric.foo",
			expectedValue:      0.0,
			expectedTags:       []string{},
			expectedSubMetrics: 1,
		},
		{
			caseName:           "Forced flag_stream invalid index",
			metricName:         "metric",
			value:              valuestore.ResultValue{Value: "1010"},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            checkconfig.MetricsConfigOption{Placement: 10, MetricSuffix: "foo"},
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
			value:              valuestore.ResultValue{Value: valuestore.ResultValue{}},
			tags:               []string{},
			forcedType:         "flag_stream",
			options:            checkconfig.MetricsConfigOption{Placement: 10, MetricSuffix: "foo"},
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
			caseName:           "Unsupported type",
			metricName:         "gauge.metric",
			value:              valuestore.ResultValue{Value: "1"},
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
			value:               valuestore.ResultValue{SubmissionType: "gauge", Value: string("22C")},
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
			value:               valuestore.ResultValue{SubmissionType: "gauge", Value: string("NOMATCH")},
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
			metricSender := MetricSender{sender: mockSender}
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
		{
			name: "report column error",
			metrics: []checkconfig.MetricsConfig{
				{
					ForcedType: "monotonic_count",
					Symbols: []checkconfig.SymbolConfig{
						{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
						{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
					},
					MetricTags: []checkconfig.MetricTagConfig{
						{Tag: "interface", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
						{Tag: "interface_alias", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
					},
				},
			},
			values: &valuestore.ResultValueStore{},
			expectedLogs: []logCount{
				{"[DEBUG] reportColumnMetrics: report column: error getting column value: value for Column OID `1.3.6.1.2.1.2.2.1.13` not found in results", 1},
				{"[DEBUG] reportColumnMetrics: report column: error getting column value: value for Column OID `1.3.6.1.2.1.2.2.1.14` not found in results", 1},
			},
		},
		{
			name: "report column cache",
			metrics: []checkconfig.MetricsConfig{
				{
					ForcedType: "monotonic_count",
					Symbols: []checkconfig.SymbolConfig{
						{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
						{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
					},
					MetricTags: []checkconfig.MetricTagConfig{
						{Tag: "interface", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
					},
				},
			},
			values: &valuestore.ResultValueStore{
				ColumnValues: valuestore.ColumnResultValuesType{
					"1.3.6.1.2.1.2.2.1.14": map[string]valuestore.ResultValue{
						"10": {
							SubmissionType: "gauge",
							Value:          10,
						},
					},
					"1.3.6.1.2.1.2.2.1.13": map[string]valuestore.ResultValue{
						"10": {
							SubmissionType: "gauge",
							Value:          10,
						},
					},
					"1.3.6.1.2.1.31.1.1.1.1": map[string]valuestore.ResultValue{
						"10": {
							Value: "myIfName",
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
			name: "report scalar error",
			metricsTags: []checkconfig.MetricTagConfig{
				{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
				{Tag: "snmp_host", OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
			},
			values: &valuestore.ResultValueStore{},
			expectedLogs: []logCount{
				{"[DEBUG] GetCheckInstanceMetricTags: metric tags: error getting scalar value: value for Scalar OID `1.2.3` not found in results", 1},
				{"[DEBUG] GetCheckInstanceMetricTags: metric tags: error getting scalar value: value for Scalar OID `1.3.6.1.2.1.1.5.0` not found in results", 1},
			},
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
