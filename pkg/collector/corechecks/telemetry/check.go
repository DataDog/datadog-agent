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

type checkImpl struct {
	corechecks.CheckBase
	telemetry telemetry.Component
}

func (c *checkImpl) Run() error {
	mfs, err := c.telemetry.Gather(true)
	if err != nil {
		return err
	}

	pointTelemetry := collectPointTelemetry(mfs, false)
	remoteMfs, err := c.telemetry.Gather(false)
	if err != nil {
		log.Warnf("failed to gather remote agent telemetry metrics for point telemetry merge: %v", err)
	} else {
		pointTelemetry.merge(collectPointTelemetry(remoteMfs, true))
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.SetNoIndex(true)

	c.sendPointTelemetry(pointTelemetry, sender)
	c.handleMetricFamilies(mfs, sender)

	return nil
}

type pointTelemetryByDomain struct {
	sent    map[string]float64
	dropped map[string]float64
}

func newPointTelemetryByDomain() pointTelemetryByDomain {
	return pointTelemetryByDomain{
		sent:    make(map[string]float64),
		dropped: make(map[string]float64),
	}
}

func (p pointTelemetryByDomain) merge(other pointTelemetryByDomain) {
	for domain, value := range other.sent {
		p.sent[domain] += value
	}
	for domain, value := range other.dropped {
		p.dropped[domain] += value
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

func collectPointTelemetry(mfs []*dto.MetricFamily, requireRemoteAgent bool) pointTelemetryByDomain {
	points := newPointTelemetryByDomain()

	for _, mf := range mfs {
		if mf == nil || mf.Name == nil || mf.Type == nil {
			continue
		}

		var pointsByDomain map[string]float64
		switch mf.GetName() {
		case pointSentMetric:
			pointsByDomain = points.sent
		case pointDroppedMetric:
			pointsByDomain = points.dropped
		default:
			continue
		}

		if mf.GetType() != dto.MetricType_GAUGE {
			log.Warnf("dropping point telemetry metric %q with unsupported type %s", mf.GetName(), mf.GetType())
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
			domain, _ := labelValue(metric.Label, domainLabel)
			pointsByDomain[domain] += metric.Gauge.GetValue()
		}
	}

	return points
}

func (c *checkImpl) sendPointTelemetry(points pointTelemetryByDomain, sender sender.Sender) {
	for domain, value := range points.sent {
		sender.Gauge(c.buildName(pointSentMetric), value, "", []string{fmt.Sprintf("%s:%s", domainLabel, domain)})
	}
	for domain, value := range points.dropped {
		sender.Gauge(c.buildName(pointDroppedMetric), value, "", []string{fmt.Sprintf("%s:%s", domainLabel, domain)})
	}
}

func (c *checkImpl) handleMetricFamilies(mfs []*dto.MetricFamily, sender sender.Sender) {
	for _, mf := range mfs {
		if mf.Name == nil || mf.Type == nil || len(mf.Metric) == 0 || isPointTelemetryMetric(mf.GetName()) {
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

func isPointTelemetryMetric(name string) bool {
	return name == pointSentMetric || name == pointDroppedMetric
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
