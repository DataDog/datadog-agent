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
)

// List of metrics to scrape out of the internal telemetry registry.
//
// This list is _deliberately_ small, and well documented: these metrics are always sent to the customer's organization,
// so they're incurring the egress cost for these metrics, no matter how slight, and they are sent for good reason. We should
// be extremely mindful both of what we add _and_ what we remove.
var defaultMetrics = []string{
	// Powers the "HA Agent Overview" out-of-the-box dashboard in customer accounts.
	"ha_agent__integration_runs",

	// Count of points sent/dropped from the perspective of the forwarder.
	//
	// Not used to power any user experiences, but referenced heavily in customer resources, such as monitors and dashboards.
	// Simply put, we don't want to cause customer monitors to fire because we removed a metric. C'est la vie.
	"point__sent",
	"point__dropped",
}

type checkImpl struct {
	corechecks.CheckBase
	telemetry telemetry.Component
}

func (c *checkImpl) Run() error {
	mfs, err := c.telemetry.Gather(telemetry.StaticMetricFilter(defaultMetrics...))
	if err != nil {
		log.Warnf("agent_telemetry check: failed to gather default telemetry metrics: %v", err)
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.SetNoIndex(true)

	c.handleMetricFamilies(mfs, sender)

	return nil
}

func (c *checkImpl) handleMetricFamilies(mfs []*dto.MetricFamily, sender sender.Sender) {
	for _, mf := range mfs {
		if mf == nil || mf.Name == nil || mf.Type == nil || len(mf.Metric) == 0 {
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
