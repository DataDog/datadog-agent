// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry is a check to collect and send limited subset of internal telemetry from the
// core agent. The check implements a subset of openmetrics v2 check functionality.
package telemetry

import (
	"fmt"
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

// regularRegistryMergeMetric describes a gauge metric from the regular telemetry registry that should be folded into
// the customer-facing default telemetry output. The metric is aggregated by groupByLabel so the emitted tag shape stays
// compatible with the existing default metric.
type regularRegistryMergeMetric struct {
	name         string
	groupByLabel string
}

// regularRegistryMergeMetrics is intentionally small and explicit: regular registry telemetry is internal by default,
// and only metrics listed here are merged into customer-facing datadog.agent.* telemetry.
var regularRegistryMergeMetrics = []regularRegistryMergeMetric{
	{name: pointSentMetric, groupByLabel: domainLabel},
	{name: pointDroppedMetric, groupByLabel: domainLabel},
}

type checkImpl struct {
	corechecks.CheckBase
	telemetry telemetry.Component
}

func (c *checkImpl) Run() error {
	mfs, err := c.telemetry.Gather(true)
	if err != nil {
		return err
	}

	mergedMetrics := collectMergeMetrics(mfs, false)

	// Remote Agent Registry telemetry lives in the regular registry. Gather it on a best-effort basis so failures there
	// do not prevent the default customer-facing telemetry check from reporting Core Agent values.
	regularMfs, err := c.telemetry.Gather(false)
	if err != nil {
		log.Warnf("failed to gather regular telemetry metrics for default telemetry merge: %v", err)
	} else {
		mergedMetrics.merge(collectMergeMetrics(regularMfs, true))
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.SetNoIndex(true)

	c.sendMergedMetrics(mergedMetrics, sender)
	c.handleMetricFamilies(mfs, sender)

	return nil
}

type mergeMetricValues map[string]map[string]float64

func newMergeMetricValues() mergeMetricValues {
	return make(mergeMetricValues)
}

func (m mergeMetricValues) merge(other mergeMetricValues) {
	for metricName, otherByGroup := range other {
		byGroup := m[metricName]
		if byGroup == nil {
			byGroup = make(map[string]float64)
			m[metricName] = byGroup
		}
		for groupValue, value := range otherByGroup {
			byGroup[groupValue] += value
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

func mergeMetricConfig(name string) (regularRegistryMergeMetric, bool) {
	for _, metric := range regularRegistryMergeMetrics {
		if metric.name == name {
			return metric, true
		}
	}
	return regularRegistryMergeMetric{}, false
}

func collectMergeMetrics(mfs []*dto.MetricFamily, requireRemoteAgent bool) mergeMetricValues {
	values := newMergeMetricValues()

	for _, mf := range mfs {
		if mf == nil || mf.Name == nil || mf.Type == nil {
			continue
		}

		mergeMetric, ok := mergeMetricConfig(mf.GetName())
		if !ok {
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
			groupValue, _ := labelValue(metric.Label, mergeMetric.groupByLabel)
			byGroup := values[mergeMetric.name]
			if byGroup == nil {
				byGroup = make(map[string]float64)
				values[mergeMetric.name] = byGroup
			}
			byGroup[groupValue] += metric.Gauge.GetValue()
		}
	}

	return values
}

func (c *checkImpl) sendMergedMetrics(values mergeMetricValues, sender sender.Sender) {
	for _, mergeMetric := range regularRegistryMergeMetrics {
		byGroup := values[mergeMetric.name]
		for groupValue, value := range byGroup {
			sender.Gauge(c.buildName(mergeMetric.name), value, "", []string{fmt.Sprintf("%s:%s", mergeMetric.groupByLabel, groupValue)})
		}
	}
}

func (c *checkImpl) handleMetricFamilies(mfs []*dto.MetricFamily, sender sender.Sender) {
	for _, mf := range mfs {
		// Merged metrics are emitted explicitly by sendMergedMetrics so regular-registry values can be included without
		// changing the customer-facing metric names or tags.
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

func isMergedMetric(name string) bool {
	_, ok := mergeMetricConfig(name)
	return ok
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
