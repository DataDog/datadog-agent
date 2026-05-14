// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry is a check to collect and send limited subset of internal telemetry from the
// core agent. The check implements a subset of openmetrics v2 check functionality.
package telemetry

import (
	"fmt"
	"slices"
	"strings"

	dto "github.com/prometheus/client_model/go"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "telemetry"
	prefix    = "datadog.agent."

	domainLabel      = "domain"
	remoteAgentLabel = "remote_agent"

	pointSentMetric    = "point__sent"
	pointDroppedMetric = "point__dropped"
)

// regularRegistryMergeMetrics is intentionally small and explicit: regular registry telemetry is internal by default.
// Some remote agents, such as ADP, emit telemetry that overlaps with Core Agent default telemetry; only metrics listed
// here are folded into customer-facing datadog.agent.* telemetry.
var regularRegistryMergeMetrics = []string{pointSentMetric, pointDroppedMetric}

type checkImpl struct {
	corechecks.CheckBase
	telemetry telemetry.Component
}

func (c *checkImpl) Run() error {
	mfs, err := c.telemetry.Gather(true)
	if err != nil {
		return err
	}

	// Remote Agent Registry telemetry lives in the regular registry. Gather it on a best-effort basis so failures there
	// do not prevent the customer-facing telemetry check from reporting Core Agent default telemetry values.
	var regularMfs []*dto.MetricFamily
	if gathered, err := c.telemetry.Gather(false); err != nil {
		log.Warnf("failed to gather regular telemetry metrics for default telemetry merge: %v", err)
	} else {
		regularMfs = gathered
	}

	mergeLabelsByMetric := discoverMergeLabels(mfs, regularMfs)
	mergedMetrics := collectMergeMetrics(mfs, false, mergeLabelsByMetric)
	mergedMetrics.merge(collectMergeMetrics(regularMfs, true, mergeLabelsByMetric))

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.SetNoIndex(true)

	c.sendMergedMetrics(mergedMetrics, sender)
	c.handleMetricFamilies(mfs, sender)

	return nil
}

type mergeMetricSample struct {
	tags  []string
	value float64
}

type mergeMetricValues map[string]map[string]mergeMetricSample

func newMergeMetricValues() mergeMetricValues {
	return make(mergeMetricValues)
}

func mergeKey(tags []string) string {
	return strings.Join(tags, "\xff")
}

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

func isMergedMetric(name string) bool {
	return slices.Contains(regularRegistryMergeMetrics, name)
}

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
				if name == "" || name == remoteAgentLabel {
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

func discoverMergeLabels(defaultMfs, regularMfs []*dto.MetricFamily) map[string][]string {
	labelsByMetric := make(map[string][]string, len(regularRegistryMergeMetrics))
	for _, metricName := range regularRegistryMergeMetrics {
		labels := mergeLabelNames(defaultMfs, metricName)
		if len(labels) == 0 {
			// Prefer the default registry's label shape for compatibility. If it has no samples yet, fall back to the
			// regular registry while still dropping remote_agent so customer-facing tags do not include attribution.
			labels = mergeLabelNames(regularMfs, metricName)
		}
		labelsByMetric[metricName] = labels
	}
	return labelsByMetric
}

func mergeTags(labels []*dto.LabelPair, labelNames []string) []string {
	tags := make([]string, 0, len(labelNames))
	for _, labelName := range labelNames {
		value, _ := labelValue(labels, labelName)
		tags = append(tags, fmt.Sprintf("%s:%s", labelName, value))
	}
	return tags
}

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
				if _, ok := labelValue(metric.Label, remoteAgentLabel); !ok {
					continue
				}
			}
			values.add(mf.GetName(), mergeTags(metric.Label, labelsByMetric[mf.GetName()]), metric.Gauge.GetValue())
		}
	}

	return values
}

func (c *checkImpl) sendMergedMetrics(values mergeMetricValues, sender sender.Sender) {
	for _, metricName := range regularRegistryMergeMetrics {
		for _, sample := range values[metricName] {
			sender.Gauge(c.buildName(metricName), sample.value, "", sample.tags)
		}
	}
}

func (c *checkImpl) handleMetricFamilies(mfs []*dto.MetricFamily, sender sender.Sender) {
	for _, mf := range mfs {
		// Merged metrics are emitted explicitly by sendMergedMetrics so overlapping regular-registry values can be included
		// without changing customer-facing metric names or tags.
		if mf == nil || mf.Name == nil || mf.Type == nil || len(mf.Metric) == 0 || isMergedMetric(mf.GetName()) {
			continue
		}

		name := c.buildName(*mf.Name)

		for _, m := range mf.Metric {
			if m == nil {
				continue
			}

			tags := c.buildTags(m.Label)

			switch *mf.Type {
			case dto.MetricType_GAUGE:
				if m.Gauge == nil {
					continue
				}
				sender.Gauge(name, *m.Gauge.Value, "", tags)
			case dto.MetricType_COUNTER:
				if m.Counter == nil {
					continue
				}
				sender.MonotonicCountWithFlushFirstValue(name, *m.Counter.Value, "", tags, true)
			default:
				log.Debugf("unknown telemetry metric type: %s", mf)
			}
		}
	}

	sender.Commit()
}

func (c *checkImpl) buildName(name string) string {
	return prefix + strings.ReplaceAll(name, "__", ".")
}

func (c *checkImpl) buildTags(lps []*dto.LabelPair) []string {
	out := make([]string, 0, len(lps))

	for _, lp := range lps {
		if lp.Name == nil {
			continue
		}
		if lp.Value == nil {
			out = append(out, *lp.Name)
		} else {
			out = append(out, fmt.Sprintf("%s:%s", *lp.Name, *lp.Value))
		}
	}

	return out
}

// Factory creates a new check factory
func Factory(telemetry telemetry.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &checkImpl{
			CheckBase: corechecks.NewCheckBase(CheckName),
			telemetry: telemetry,
		}
	})
}
