// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry is a check to collect and send limited subset of internal telemetry from the
// core agent. The check implements a subset of openmetrics v2 check functionality.
package telemetry

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	dto "github.com/prometheus/client_model/go"

	"github.com/DataDog/datadog-agent/comp/core/config"
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

	// internalTelemetryEnabledSetting reports the curated set of internal telemetry metrics, and
	// internalTelemetryAdvancedSetting widens that to the whole registry. Both are runtime
	// settings, so they are read on every run rather than cached at configuration time.
	internalTelemetryEnabledSetting  = "telemetry.internal.enabled"
	internalTelemetryAdvancedSetting = "telemetry.internal.advanced"
)

type checkImpl struct {
	corechecks.CheckBase
	telemetry telemetry.Component
	config    config.Component
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
	internalTelemetryAdvanced := c.config.GetBool(internalTelemetryAdvancedSetting)
	internalTelemetryEnabled := internalTelemetryAdvanced || c.config.GetBool(internalTelemetryEnabledSetting)
	if internalTelemetryEnabled {
		c.sendNonDefaultTelemetry(regularMfs, internalTelemetryAdvanced, defaultReportedMetrics, sender)
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

func (c *checkImpl) sendNonDefaultTelemetry(mfs []*dto.MetricFamily, advanced bool, reported reportedMetrics, sender sender.Sender) {
	// Set up an additional filter that includes certain non-default metrics depending on whether or not advanced
	// internal telemetry is enabled.
	shouldInclude := telemetry.NoFilter
	if !advanced {
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
				sendHistogramAsDistribution(name, m.Histogram, tags, sender)
			}
		}
	}
}

// sendHistogramAsDistribution reports a Prometheus histogram as a native Datadog distribution,
// feeding each bucket range to the check sampler which interpolates them into a sketch. This is
// what the Python openmetrics check produces with `send_distribution_buckets: true`, and it costs
// one series per histogram rather than one per bucket.
//
// Prometheus bucket counts are cumulative in two independent senses, and only one of them is the
// sampler's job:
//
//   - Across buckets: `le=10` counts everything already counted by `le=1`. De-cumulated here,
//     since submitting the raw counts would credit the (1,10] range with observations belonging
//     to [0,1].
//   - Over time: every count grows for the lifetime of the process. Handled by the sampler,
//     which diffs against the previous run per (context, bucket bounds) given Monotonic: true.
//     De-cumulated per-bucket counts are still monotonic over time, so that stays correct.
//
// Note that the sampler drops a bucket whose value is zero before it records a previous value for
// it, so the first observation to land in a previously-empty bucket is lost. Passing
// flushFirstValue would avoid that, at the cost of reporting everything since Agent start as if
// it happened in one interval.
func sendHistogramAsDistribution(name string, histogram *dto.Histogram, tags []string, sender sender.Sender) {
	// Sort defensively. client_golang emits buckets in ascending order, but remote agent
	// telemetry is reparsed from the text format, and an out-of-order bucket would produce
	// negative deltas that the sampler discards.
	buckets := slices.Clone(histogram.Bucket)
	slices.SortFunc(buckets, func(a, b *dto.Bucket) int {
		return cmp.Compare(a.GetUpperBound(), b.GetUpperBound())
	})

	// The sampler treats an infinite upper bound as a point mass at the lower bound, so the
	// overflow bucket is reported like any other.
	var lowerBound float64
	var prevCumulative uint64
	sawInf := false

	for _, bucket := range buckets {
		if bucket == nil {
			continue
		}

		upperBound := bucket.GetUpperBound()
		cumulative := bucket.GetCumulativeCount()
		if cumulative < prevCumulative {
			// Not de-cumulatable: the family is malformed rather than merely unsorted.
			log.Debugf("skipping out-of-order bucket [%f-%f] for telemetry metric %q", lowerBound, upperBound, name)
			continue
		}

		sender.HistogramBucket(name, int64(cumulative-prevCumulative), lowerBound, upperBound, true, "", tags, false)

		sawInf = sawInf || math.IsInf(upperBound, 1)
		lowerBound = upperBound
		prevCumulative = cumulative
	}

	// client_golang leaves the +Inf bucket out of the dto entirely and carries its count in
	// SampleCount, while the text parser used for remote agent telemetry includes it. Synthesize
	// it when it is missing, otherwise the tail of every Core Agent histogram is dropped.
	if !sawInf {
		if total := histogram.GetSampleCount(); total > prevCumulative {
			sender.HistogramBucket(name, int64(total-prevCumulative), lowerBound, math.Inf(1), true, "", tags, false)
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
func Factory(telemetry telemetry.Component, config config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &checkImpl{
			CheckBase: corechecks.NewCheckBase(CheckName),
			telemetry: telemetry,
			config:    config,
		}
	})
}
