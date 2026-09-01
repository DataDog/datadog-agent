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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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
	mfs, err := c.telemetry.Gather(true)
	if err != nil {
		log.Warnf("agent_telemetry check: failed to gather default telemetry metrics: %v", err)
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

	if c.cfg.InternalTelemetry.Enabled {
		// Internal telemetry stands in for the go_expvar agent_stats instance, whose metrics are
		// indexed, so these are reported as regular indexed metrics rather than no-index ones.
		sender.SetNoIndex(false)
		c.sendInternalTelemetry(regularMfs, sender)
	}

	sender.Commit()

	return nil
}

func (c *checkImpl) handleMetricFamilies(mfs []*dto.MetricFamily, sender sender.Sender) {
	for _, mf := range mfs {
		// Merged metrics are emitted explicitly by sendMergedMetrics so overlapping regular-registry values can be included
		// without changing customer-facing metric names or tags.
		if mf == nil || isMergedMetric(mf.GetName()) {
			continue
		}

		c.sendMetricFamily(mf, sender)
	}
}

// sendInternalTelemetry reports the regular telemetry registry: either the curated set that gives
// parity with the go_expvar agent_stats example, or everything when running in advanced mode.
func (c *checkImpl) sendInternalTelemetry(mfs []*dto.MetricFamily, sender sender.Sender) {
	include := telemetry.NoFilter
	if !c.cfg.InternalTelemetry.Advanced {
		include = telemetry.StaticMetricFilter(curatedInternalMetrics...)
	}

	for _, mf := range mfs {
		// sendMergedMetrics already reports these, folding the regular-registry remote agent
		// series into the default-registry ones. Reporting them again here would double count.
		if mf == nil || isMergedMetric(mf.GetName()) || !include(mf) {
			continue
		}

		c.sendMetricFamily(mf, sender)
	}
}

// sendMetricFamily reports every series of a single Prometheus metric family. Summaries and
// untyped metrics are not supported and are dropped.
func (c *checkImpl) sendMetricFamily(mf *dto.MetricFamily, sender sender.Sender) {
	if mf.Name == nil || mf.Type == nil || len(mf.Metric) == 0 {
		return
	}

	name := c.buildName(mf.GetName())

	for _, m := range mf.Metric {
		if m == nil {
			continue
		}

		tags := c.buildTags(m.Label)

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
		default:
			log.Debugf("unsupported telemetry metric type %s for metric %q", mf.GetType(), mf.GetName())
		}
	}
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
