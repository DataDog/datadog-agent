// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	dto "github.com/prometheus/client_model/go"
)

const (
	emitterLabel = "emitter"

	pointSentMetric    = "point__sent"
	pointDroppedMetric = "point__dropped"
)

// regularRegistryMergeMetrics is intentionally small and explicit: regular registry telemetry is internal by default.
// Some remote agents, such as ADP, emit telemetry that overlaps with Core Agent default telemetry; only metrics listed
// here are folded into customer-facing datadog.agent.* telemetry.
var regularRegistryMergeMetrics = []string{pointSentMetric, pointDroppedMetric}

type mergeMetricSample struct {
	tags  []string
	value float64
}

// mergeMetricValues stores merge candidates by metric name and customer-facing tag set.
type mergeMetricValues map[string]map[string]mergeMetricSample

func newMergeMetricValues() mergeMetricValues {
	return make(mergeMetricValues, len(regularRegistryMergeMetrics))
}

// mergeKey builds a stable key for a customer-facing tag set.
func mergeKey(tags []string) string {
	return strings.Join(tags, "\xff")
}

// add accumulates a metric value into the bucket identified by metric name and tag set.
func (m mergeMetricValues) add(metricName string, tags []string, value float64) {
	byKey := m[metricName]
	if byKey == nil {
		byKey = make(map[string]mergeMetricSample)
		m[metricName] = byKey
	}

	key := mergeKey(tags)
	sample := byKey[key]
	if sample.tags == nil {
		sample.tags = tags
	}
	sample.value += value
	byKey[key] = sample
}

// merge folds another merge set into the receiver.
func (m mergeMetricValues) merge(other mergeMetricValues) {
	for metricName, otherByKey := range other {
		for _, sample := range otherByKey {
			m.add(metricName, sample.tags, sample.value)
		}
	}
}

func labelValue(labels []*dto.LabelPair, name string) (string, bool) {
	for _, label := range labels {
		if label.GetName() == name {
			return label.GetValue(), true
		}
	}
	return "", false
}

// isMergedMetric returns true when a default-registry metric should be emitted through the explicit merge path.
func isMergedMetric(name string) bool {
	return slices.Contains(regularRegistryMergeMetrics, name)
}

// mergeLabelNames returns the sorted label names used by the given metric family, excluding emitter.
func mergeLabelNames(mfs []*dto.MetricFamily, metricName string) []string {
	labelNames := make(map[string]struct{})
	for _, mf := range mfs {
		if mf == nil || mf.GetName() != metricName {
			continue
		}
		for _, metric := range mf.Metric {
			if metric == nil {
				continue
			}
			for _, label := range metric.Label {
				name := label.GetName()
				if name == "" || name == emitterLabel {
					continue
				}
				labelNames[name] = struct{}{}
			}
		}
	}

	names := make([]string, 0, len(labelNames))
	for name := range labelNames {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// discoverMergeLabels determines the customer-facing tag shape for each merged metric.
//
// The default registry is preferred because it defines the existing customer-facing metric shape. If the default
// registry has no samples yet, regular-registry labels are used as a fallback while still dropping emitter.
func discoverMergeLabels(defaultMfs, regularMfs []*dto.MetricFamily) map[string][]string {
	labelsByMetric := make(map[string][]string, len(regularRegistryMergeMetrics))
	for _, metricName := range regularRegistryMergeMetrics {
		labels := mergeLabelNames(defaultMfs, metricName)
		if len(labels) == 0 {
			labels = mergeLabelNames(regularMfs, metricName)
		}
		labelsByMetric[metricName] = labels
	}
	return labelsByMetric
}

// mergeTags builds customer-facing tags from the selected label names, using an empty value for missing labels.
func mergeTags(labels []*dto.LabelPair, labelNames []string) []string {
	tags := make([]string, 0, len(labelNames))
	for _, labelName := range labelNames {
		value, _ := labelValue(labels, labelName)
		tags = append(tags, labelName+":"+value)
	}
	return tags
}

// collectMergeMetrics extracts allowlisted gauge metrics and aggregates them by the discovered customer-facing tags.
//
// When requireRemoteAgent is true, only series with a emitter label are collected. This prevents unrelated regular
// registry series from being folded into customer-facing telemetry.
func collectMergeMetrics(mfs []*dto.MetricFamily, requireRemoteAgent bool, labelsByMetric map[string][]string) mergeMetricValues {
	values := newMergeMetricValues()

	for _, mf := range mfs {
		if mf == nil || mf.Name == nil || mf.Type == nil || !isMergedMetric(mf.GetName()) {
			continue
		}

		if mf.GetType() != dto.MetricType_GAUGE {
			log.Warnf("dropping telemetry merge metric %q with unsupported type %s", mf.GetName(), mf.GetType())
			continue
		}

		for _, metric := range mf.Metric {
			if metric == nil || metric.Gauge == nil {
				continue
			}
			if requireRemoteAgent {
				if _, ok := labelValue(metric.Label, emitterLabel); !ok {
					continue
				}
			}
			values.add(mf.GetName(), mergeTags(metric.Label, labelsByMetric[mf.GetName()]), metric.Gauge.GetValue())
		}
	}

	return values
}

// sendMergedMetrics emits metrics that combine default-registry values with overlapping regular-registry values.
func (c *checkImpl) sendMergedMetrics(values mergeMetricValues, sender sender.Sender) {
	for _, metricName := range regularRegistryMergeMetrics {
		for _, sample := range values[metricName] {
			sender.Gauge(c.buildName(metricName), sample.value, "", sample.tags)
		}
	}
}
