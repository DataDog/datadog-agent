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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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

	// seriesKeySep is a byte that cannot occur in a metric name or tag, so joining on it cannot
	// make two different series produce the same key.
	seriesKeySep = "\xff"
)

type checkImpl struct {
	corechecks.CheckBase
	telemetry telemetry.Component
	cfg       instanceConfig
}

// Configure parses the instance configuration before handing off to the common check setup.
func (c *checkImpl) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	cfg, err := parseInstanceConfig(data)
	if err != nil {
		return err
	}
	c.cfg = cfg

	return c.CheckBase.Configure(senderManager, integrationConfigDigest, data, initConfig, source, provider)
}

func (c *checkImpl) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// Gather all metrics from both the default and non-default registries.
	//
	// We _fallibly_ gather telemetry from the non-default registry since it doesn't represent the baseline that we
	// _always_ want to send, and exiting the check run early due to failure would mean we skip emitting any default
	// metrics we managed to collect.
	defaultMfs, err := c.telemetry.Gather(true)
	if err != nil {
		log.Warnf("telemetry check: failed to gather default telemetry metrics: %v", err)
		return err
	}

	var regularMfs []*dto.MetricFamily
	if gathered, err := c.telemetry.Gather(false); err != nil {
		log.Warnf("telemetry check: failed to gather regular telemetry metrics: %v", err)
	} else {
		regularMfs = gathered
	}

	// Now send all default metrics, including any non-default metrics that overlap (by name) with any default metrics.
	//
	// This ensures that if any default metrics are being partially or solely populated by remote agents (via RAR, which
	// feeds telemetry into the _non-default_ registry) that we drag them along into our batch of default metrics. We
	// also track which non-default metrics got groupedand sent this way, and exclude them in any subsequent sends of
	// non-default metrics for this check run.
	defaultReportedMetrics := buildReportedMetricsFromMetricFamilies(defaultMfs)

	c.sendDefaultTelemetry(defaultMfs, sender)
	c.sendOverlappedNonDefaultTelemetry(regularMfs, defaultReportedMetrics, sender)

	// If "internal telemetry" is enabled, we send the remainder of the non-default metrics, which may be further
	// filtered depending on whether or not "advanced" mode is enabled.
	if c.cfg.InternalTelemetry.Enabled {
		c.sendNonDefaultTelemetry(regularMfs, defaultReportedMetrics, sender)
	}

	sender.Commit()

	return nil
}

// reportedMetrics tracks the unique set of metric names, and metric series, based on an input set of Prometheus metric
// families.
type reportedMetrics struct {
	names  map[string]struct{}
	series map[string]struct{}
}

func (rm *reportedMetrics) hasOverlap(mf *dto.MetricFamily) bool {
	_, overlap := rm.names[mf.GetName()]
	return overlap
}

func (rm *reportedMetrics) isDuplicate(mf *dto.MetricFamily, tags []string) bool {
	_, duplicate := rm.series[seriesKey(mf.GetName(), tags)]
	return duplicate
}

// buildReportedMetricsFromMetricFamilies builds a list of all unique metric names, and series, within a given set of
// metric families.
//
// This is used to calculate the intersection or difference between two sets of metric families, for the purpose of
// either sending a non-default metric that matches a default metric, or excluding such a non-default metric from being
// included multiple times in subsequent sends.
func buildReportedMetricsFromMetricFamilies(mfs []*dto.MetricFamily) reportedMetrics {
	reported := reportedMetrics{
		names:  make(map[string]struct{}, len(mfs)),
		series: make(map[string]struct{}, len(mfs)),
	}

	for _, mf := range mfs {
		if !reportableMetricFamily(mf) {
			continue
		}

		reported.names[mf.GetName()] = struct{}{}
		for _, m := range mf.Metric {
			if m == nil {
				continue
			}
			reported.series[seriesKey(mf.GetName(), buildTags(m.Label))] = struct{}{}
		}
	}

	return reported
}

// seriesKey identifies a single time series by metric name and full tag set. Tags are sorted so
// that two registries listing the same labels in a different order still collide.
func seriesKey(name string, tags []string) string {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)
	return name + seriesKeySep + strings.Join(sorted, seriesKeySep)
}

func (c *checkImpl) sendDefaultTelemetry(mfs []*dto.MetricFamily, sender sender.Sender) {
	c.sendMetricFamilies(mfs, nil, nil, sender)
}

func (c *checkImpl) sendOverlappedNonDefaultTelemetry(mfs []*dto.MetricFamily, reported reportedMetrics, sender sender.Sender) {
	// We need to make sure we only send overlapping metrics, but not _duplicate_ metrics.
	familyFilter := func(mf *dto.MetricFamily) bool {
		return reported.hasOverlap(mf)
	}

	metricFilter := func(mf *dto.MetricFamily, tags []string) bool {
		return !reported.isDuplicate(mf, tags)
	}

	c.sendMetricFamilies(mfs, familyFilter, metricFilter, sender)
}

func (c *checkImpl) sendNonDefaultTelemetry(mfs []*dto.MetricFamily, reported reportedMetrics, sender sender.Sender) {
	// Set up an additional filter that includes certain non-default metrics depending on whether or not advanced
	// internal telemetry is enabled.
	shouldInclude := telemetry.NoFilter
	if !c.cfg.InternalTelemetry.Advanced {
		shouldInclude = telemetry.StaticMetricFilter(curatedInternalMetrics...)
	}

	// We need to make sure we don't send any non-default metrics that have overlap with `reported`, and we include our
	// additional filtering from above.
	familyFilter := func(mf *dto.MetricFamily) bool {
		return !reported.hasOverlap(mf) && shouldInclude(mf)
	}

	c.sendMetricFamilies(mfs, familyFilter, nil, sender)
}

// sendMetricFamilies sends a set of metric family to the check aggregator
//
// `familyFilter` and `metricFilter` are predicates used to control whether or not the given metric families, or a metric
// within a family, are allowed to be sent, respectively. Predicates should return `true` if the metric family/metric
// is to be allowed through. If a predicate is `nil`, the respective predicate check is skipped.
func (c *checkImpl) sendMetricFamilies(mfs []*dto.MetricFamily, familyFilter func(*dto.MetricFamily) bool, metricFilter func(*dto.MetricFamily, []string) bool, sender sender.Sender) {
	for _, mf := range mfs {
		if mf == nil {
			continue
		}

		if familyFilter != nil && !familyFilter(mf) {
			continue
		}

		if !reportableMetricFamily(mf) {
			continue
		}

		name := buildName(mf.GetName())

		for _, m := range mf.Metric {
			if m == nil {
				continue
			}

			tags := buildTags(m.Label)

			if metricFilter != nil && !metricFilter(mf, tags) {
				continue
			}

			switch mf.GetType() {
			case dto.MetricType_GAUGE:
				if m.Gauge == nil {
					continue
				}
				sender.Gauge(name, m.Gauge.GetValue(), "", tags)
			case dto.MetricType_COUNTER:
				if m.Counter == nil {
					continue
				}
				sender.MonotonicCountWithFlushFirstValue(name, m.Counter.GetValue(), "", tags, true)
			case dto.MetricType_HISTOGRAM:
				if m.Histogram == nil {
					continue
				}
				// Buckets are dropped on purpose: `le` is unbounded and would multiply the number of
				// series by the bucket count. Sum and count still give the rate and the average.
				sender.MonotonicCountWithFlushFirstValue(name+".sum", m.Histogram.GetSampleSum(), "", tags, true)
				sender.MonotonicCountWithFlushFirstValue(name+".count", float64(m.Histogram.GetSampleCount()), "", tags, true)
			}
		}
	}
}

// reportableMetricFamily reports whether a metric family can be sent. Summary and untyped families
// are not supported and are dropped.
func reportableMetricFamily(mf *dto.MetricFamily) bool {
	if mf == nil || mf.Name == nil || mf.Type == nil || len(mf.Metric) == 0 {
		return false
	}

	switch mf.GetType() {
	case dto.MetricType_GAUGE, dto.MetricType_COUNTER, dto.MetricType_HISTOGRAM:
		return true
	default:
		log.Debugf("skipping telemetry metric %q with unsupported type %s", mf.GetName(), mf.GetType())
		return false
	}
}

func buildName(name string) string {
	return prefix + strings.ReplaceAll(name, "__", ".")
}

func buildTags(lps []*dto.LabelPair) []string {
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
