// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

// MetricSender is a wrapper around aggregator.Sender
type MetricSender struct {
	sender           aggregator.Sender
	hostname         string
	submittedMetrics int
	interfaceConfigs []snmpintegration.InterfaceConfig
}

// MetricSample is a collected metric sample with its metadata, ready to be submitted through the metric sender
type MetricSample struct {
	value      valuestore.ResultValue
	tags       []string
	symbol     checkconfig.SymbolConfig
	forcedType string
	options    checkconfig.MetricsConfigOption
}

// NewMetricSender create a new MetricSender
func NewMetricSender(sender aggregator.Sender, hostname string, interfaceConfigs []snmpintegration.InterfaceConfig) *MetricSender {
	return &MetricSender{
		sender:           sender,
		hostname:         hostname,
		interfaceConfigs: interfaceConfigs,
	}
}

// ReportMetrics reports metrics using Sender
func (ms *MetricSender) ReportMetrics(metrics []checkconfig.MetricsConfig, values *valuestore.ResultValueStore, tags []string) {
	scalarSamples := make(map[string]MetricSample)
	columnSamples := make(map[string]map[string]MetricSample)

	for _, metric := range metrics {
		if metric.IsScalar() {
			sample, err := ms.reportScalarMetrics(metric, values, tags)
			if err != nil {
				continue
			}
			if _, ok := EvaluatedSampleDependencies[sample.symbol.Name]; !ok {
				continue
			}
			scalarSamples[sample.symbol.Name] = sample
		} else if metric.IsColumn() {
			samples := ms.reportColumnMetrics(metric, values, tags)

			for name, sampleRows := range samples {
				if _, ok := EvaluatedSampleDependencies[name]; !ok {
					continue
				}
				columnSamples[name] = sampleRows
			}
		}
	}

	err := ms.tryReportMemoryUsage(scalarSamples, columnSamples)
	if err != nil {
		log.Debugf("error reporting memory usage : %v", err)
	}
}

// GetCheckInstanceMetricTags returns check instance metric tags
func (ms *MetricSender) GetCheckInstanceMetricTags(metricTags []checkconfig.MetricTagConfig, values *valuestore.ResultValueStore) []string {
	var globalTags []string

	for _, metricTag := range metricTags {
		// TODO: Support extract value see II-635
		value, err := values.GetScalarValue(metricTag.OID)
		if err != nil {
			continue
		}
		strValue, err := value.ToString()
		if err != nil {
			log.Debugf("error converting value (%#v) to string : %v", value, err)
			continue
		}
		globalTags = append(globalTags, metricTag.GetTags(strValue)...)
	}
	return globalTags
}

func (ms *MetricSender) reportScalarMetrics(metric checkconfig.MetricsConfig, values *valuestore.ResultValueStore, tags []string) (MetricSample, error) {
	value, err := getScalarValueFromSymbol(values, metric.Symbol)
	if err != nil {
		log.Debugf("report scalar: error getting scalar value: %v", err)
		return MetricSample{}, err
	}

	scalarTags := common.CopyStrings(tags)
	scalarTags = append(scalarTags, metric.GetSymbolTags()...)
	sample := MetricSample{
		value:      value,
		tags:       scalarTags,
		symbol:     metric.Symbol,
		forcedType: metric.ForcedType,
		options:    metric.Options,
	}
	ms.sendMetric(sample)
	return sample, nil
}

func (ms *MetricSender) reportColumnMetrics(metricConfig checkconfig.MetricsConfig, values *valuestore.ResultValueStore, tags []string) map[string]map[string]MetricSample {
	rowTagsCache := make(map[string][]string)
	samples := map[string]map[string]MetricSample{}
	for _, symbol := range metricConfig.Symbols {
		metricValues, err := getColumnValueFromSymbol(values, symbol)
		if err != nil {
			continue
		}
		for fullIndex, value := range metricValues {
			// cache row tags by fullIndex to avoid rebuilding it for every column rows
			if _, ok := rowTagsCache[fullIndex]; !ok {
				tmpTags := common.CopyStrings(tags)
				tmpTags = append(tmpTags, metricConfig.StaticTags...)
				tmpTags = append(tmpTags, getTagsFromMetricTagConfigList(metricConfig.MetricTags, fullIndex, values)...)
				rowTagsCache[fullIndex] = tmpTags
			}
			rowTags := rowTagsCache[fullIndex]
			sample := MetricSample{
				value:      value,
				tags:       rowTags,
				symbol:     symbol,
				forcedType: metricConfig.ForcedType,
				options:    metricConfig.Options,
			}
			ms.sendMetric(sample)
			if _, ok := samples[sample.symbol.Name]; !ok {
				samples[sample.symbol.Name] = make(map[string]MetricSample)
			}
			samples[sample.symbol.Name][fullIndex] = sample
			ms.sendInterfaceVolumeMetrics(symbol, fullIndex, values, rowTags)
		}
	}
	return samples
}

func (ms *MetricSender) sendMetric(metricSample MetricSample) {
	metricFullName := "snmp." + metricSample.symbol.Name
	forcedType := metricSample.forcedType
	if forcedType == "" {
		if metricSample.value.SubmissionType != "" {
			forcedType = metricSample.value.SubmissionType
		} else {
			forcedType = "gauge"
		}
	} else if forcedType == "flag_stream" {
		strValue, err := metricSample.value.ToString()
		if err != nil {
			log.Debugf("error converting value (%#v) to string : %v", metricSample.value, err)
			return
		}
		options := metricSample.options
		floatValue, err := getFlagStreamValue(options.Placement, strValue)
		if err != nil {
			log.Debugf("metric `%s`: failed to get flag stream value: %s", metricFullName, err)
			return
		}
		metricFullName = metricFullName + "." + options.MetricSuffix
		metricSample.value = valuestore.ResultValue{Value: floatValue}
		forcedType = "gauge"
	}

	floatValue, err := metricSample.value.ToFloat64()
	if err != nil {
		log.Debugf("metric `%s`: failed to convert to float64: %s", metricFullName, err)
		return
	}

	scaleFactor := metricSample.symbol.ScaleFactor
	if scaleFactor != 0 {
		floatValue *= scaleFactor
	}

	switch forcedType {
	case "gauge":
		ms.Gauge(metricFullName, floatValue, metricSample.tags)
		ms.submittedMetrics++
	case "counter":
		ms.Rate(metricFullName, floatValue, metricSample.tags)
		ms.submittedMetrics++
	case "percent":
		ms.Rate(metricFullName, floatValue*100, metricSample.tags)
		ms.submittedMetrics++
	case "monotonic_count":
		ms.MonotonicCount(metricFullName, floatValue, metricSample.tags)
		ms.submittedMetrics++
	case "monotonic_count_and_rate":
		ms.MonotonicCount(metricFullName, floatValue, metricSample.tags)
		ms.Rate(metricFullName+".rate", floatValue, metricSample.tags)
		ms.submittedMetrics += 2
	default:
		log.Debugf("metric `%s`: unsupported forcedType: %s", metricFullName, forcedType)
		return
	}
}

// Gauge wraps Sender.Gauge
func (ms *MetricSender) Gauge(metric string, value float64, tags []string) {
	ms.sender.Gauge(metric, value, ms.hostname, tags)
}

// Rate wraps Sender.Rate
func (ms *MetricSender) Rate(metric string, value float64, tags []string) {
	ms.sender.Rate(metric, value, ms.hostname, tags)
}

// MonotonicCount wraps Sender.MonotonicCount
func (ms *MetricSender) MonotonicCount(metric string, value float64, tags []string) {
	ms.sender.MonotonicCount(metric, value, ms.hostname, tags)
}

// ServiceCheck wraps Sender.ServiceCheck
func (ms *MetricSender) ServiceCheck(checkName string, status metrics.ServiceCheckStatus, tags []string, message string) {
	ms.sender.ServiceCheck(checkName, status, ms.hostname, tags, message)
}

// GetSubmittedMetrics returns submitted metrics count
func (ms *MetricSender) GetSubmittedMetrics() int {
	return ms.submittedMetrics
}

func getFlagStreamValue(placement uint, strValue string) (float64, error) {
	index := placement - 1
	if int(index) >= len(strValue) {
		return 0, fmt.Errorf("flag stream index `%d` not found in `%s`", index, strValue)
	}
	charAtIndex := strValue[index]
	floatValue := 0.0
	if charAtIndex == '1' {
		floatValue = 1.0
	}
	return floatValue, nil
}
