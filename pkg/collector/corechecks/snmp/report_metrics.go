package snmp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type metricSender struct {
	sender           aggregator.Sender
	submittedMetrics int
}

func (ms *metricSender) reportMetrics(metrics []metricsConfig, values *resultValueStore, tags []string) {
	for _, metric := range metrics {
		if metric.Symbol.OID != "" {
			ms.reportScalarMetrics(metric, values, tags)
		} else if metric.Table.OID != "" {
			ms.reportColumnMetrics(metric, values, tags)
		}
	}
}

func (ms *metricSender) getCheckInstanceMetricTags(metricTags []metricTagConfig, values *resultValueStore) []string {
	var globalTags []string

	for _, metricTag := range metricTags {
		value, err := values.getScalarValue(metricTag.OID)
		if err != nil {
			log.Warnf("metric tags: error getting scalar value: %v", err)
			continue
		}
		globalTags = append(globalTags, metricTag.getTags(value.toString())...)
	}
	return globalTags
}

func (ms *metricSender) reportScalarMetrics(metric metricsConfig, values *resultValueStore, tags []string) {
	value, err := values.getScalarValue(metric.Symbol.OID)
	if err != nil {
		log.Warnf("report scalar: error getting scalar value: %v", err)
		return
	}

	scalarTags := copyStrings(tags)
	scalarTags = append(scalarTags, metric.getSymbolTags()...)
	ms.sendMetric(metric.Symbol.Name, value, scalarTags, metric.ForcedType, metric.Options)
}

func (ms *metricSender) reportColumnMetrics(metricConfig metricsConfig, values *resultValueStore, tags []string) {
	rowTagsCache := make(map[string][]string)
	for _, symbol := range metricConfig.Symbols {
		metricValues, err := values.getColumnValues(symbol.OID)
		if err != nil {
			log.Warnf("report column: error getting column value: %v", err)
			continue
		}
		for fullIndex, value := range metricValues {
			// cache row tags by fullIndex to avoid rebuilding it for every column rows
			if _, ok := rowTagsCache[fullIndex]; !ok {
				rowTagsCache[fullIndex] = append(copyStrings(tags), metricConfig.getTags(fullIndex, values)...)
				log.Debugf("report column: caching tags `%v` for fullIndex `%s`", rowTagsCache[fullIndex], fullIndex)
			}
			rowTags := rowTagsCache[fullIndex]
			ms.sendMetric(symbol.Name, value, rowTags, metricConfig.ForcedType, metricConfig.Options)
			ms.trySendBandwidthUsageMetric(symbol, fullIndex, values, rowTags)
		}
	}
}

func (ms *metricSender) sendMetric(metricName string, value snmpValueType, tags []string, forcedType string, options metricsConfigOption) {
	metricFullName := "snmp." + metricName

	// we need copy tags before using sender due to https://github.com/DataDog/datadog-agent/issues/7159
	if forcedType != "" {
		switch forcedType {
		case "gauge":
			ms.gauge(metricFullName, value.toFloat64(), "", tags)
			ms.submittedMetrics++
		case "counter":
			ms.rate(metricFullName, value.toFloat64(), "", tags)
			ms.submittedMetrics++
		case "percent":
			ms.rate(metricFullName, value.toFloat64()*100, "", tags)
			ms.submittedMetrics++
		case "monotonic_count":
			ms.monotonicCount(metricFullName, value.toFloat64(), "", tags)
			ms.submittedMetrics++
		case "monotonic_count_and_rate":
			ms.monotonicCount(metricFullName, value.toFloat64(), "", tags)
			ms.rate(metricFullName+".rate", value.toFloat64(), "", tags)
			ms.submittedMetrics += 2
		case "flag_stream":
			floatValue, err := getFlagStreamValue(options.Placement, value.toString())
			if err != nil {
				log.Debugf("metric `%s`: failed to get flag stream value: %s", metricFullName, err)
				return
			}
			ms.gauge(metricFullName+"."+options.MetricSuffix, floatValue, "", tags)
			ms.submittedMetrics++
		default:
			log.Debugf("metric `%s`: unsupported forcedType: %s", metricFullName, forcedType)
			return
		}
	} else {
		switch value.submissionType {
		case metrics.RateType:
			ms.rate(metricFullName, value.toFloat64(), "", tags)
			ms.submittedMetrics++
		default:
			ms.gauge(metricFullName, value.toFloat64(), "", tags)
			ms.submittedMetrics++
		}
	}
}

func (ms *metricSender) gauge(metric string, value float64, hostname string, tags []string) {
	// we need copy tags before using sender due to https://github.com/DataDog/datadog-agent/issues/7159
	ms.sender.Gauge(metric, value, hostname, copyStrings(tags))
}

func (ms *metricSender) rate(metric string, value float64, hostname string, tags []string) {
	// we need copy tags before using sender due to https://github.com/DataDog/datadog-agent/issues/7159
	ms.sender.Rate(metric, value, hostname, copyStrings(tags))
}

func (ms *metricSender) monotonicCount(metric string, value float64, hostname string, tags []string) {
	// we need copy tags before using sender due to https://github.com/DataDog/datadog-agent/issues/7159
	ms.sender.MonotonicCount(metric, value, hostname, copyStrings(tags))
}

func (ms *metricSender) serviceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string) {
	// we need copy tags before using sender due to https://github.com/DataDog/datadog-agent/issues/7159
	ms.sender.ServiceCheck(checkName, status, hostname, copyStrings(tags), message)
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
