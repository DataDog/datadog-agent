// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package csidriver implements the CSI driver core check.
package csidriver

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	prometheus "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// CheckName is the name of the check as registered in the loader.
const CheckName = "datadog_csi_driver"

const (
	metricNs          = "datadog.csi_driver."
	httpClientTimeout = 5 * time.Second
)

// coatMetric associates a COAT telemetry counter with the Prometheus label
// names it expects, in the order required by telemetry.Counter.Add().
type coatMetric struct {
	counter telemetry.Counter
	tagKeys []string
}

// metricDef defines how a Prometheus metric is mapped to a Datadog metric name
// and optionally to a COAT telemetry counter.
type metricDef struct {
	ddName string
	coat   *coatMetric
}

func buildMetricDefs(tm telemetry.Component) map[string]metricDef {
	return map[string]metricDef{
		"datadog_csi_driver_node_publish_volume_attempts": {
			ddName: "node_publish_volume_attempts",
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "node_publish_volume_attempts", []string{"status", "type"}, "CSI node publish volume attempts"),
				tagKeys: []string{"status", "type"},
			},
		},
		"datadog_csi_driver_node_unpublish_volume_attempts": {
			ddName: "node_unpublish_volume_attempts",
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "node_unpublish_volume_attempts", []string{"status"}, "CSI node unpublish volume attempts"),
				tagKeys: []string{"status"},
			},
		},
	}
}

// Check collects CSI driver metrics from its Prometheus endpoint.
type Check struct {
	core.CheckBase
	config     csiDriverConfig
	httpClient http.Client
	metrics    map[string]metricDef
	prevValues map[string]float64
}

// Factory creates a new check factory.
func Factory(tm telemetry.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase:  core.NewCheckBase(CheckName),
			metrics:    buildMetricDefs(tm),
			prevValues: make(map[string]float64),
		}
	})
}

// Configure parses the check configuration and initialises the HTTP client.
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return fmt.Errorf("configure %s: %w", CheckName, err)
	}

	if err := c.config.parse(config); err != nil {
		return err
	}

	c.httpClient = http.Client{Timeout: httpClientTimeout}

	log.Debugf("%s: configured (endpoint: %s)", CheckName, c.config.OpenmetricsEndpoint)

	return nil
}

// Run scrapes the CSI driver metrics endpoint and submits metrics.
func (c *Check) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get sender: %w", err)
	}
	defer s.Commit()

	metrics, err := c.scrape()
	if err != nil {
		s.ServiceCheck(metricNs+"openmetrics.health", servicecheck.ServiceCheckCritical, "", nil, fmt.Sprintf("endpoint unreachable: %s", err))
		return fmt.Errorf("scrape %s: %w", c.config.OpenmetricsEndpoint, err)
	}

	s.ServiceCheck(metricNs+"openmetrics.health", servicecheck.ServiceCheckOK, "", nil, "")

	c.submitMetrics(s, metrics)

	return nil
}

func (c *Check) scrape() ([]prometheus.MetricFamily, error) {
	resp, err := c.httpClient.Get(c.config.OpenmetricsEndpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d from %s", resp.StatusCode, c.config.OpenmetricsEndpoint)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	families, err := prometheus.ParseMetrics(body)
	if err != nil {
		return nil, fmt.Errorf("parse prometheus metrics: %w", err)
	}

	return families, nil
}

// normalizeMetricName strips the _total suffix that Prometheus client libraries
// append to counter metric names in the exposition format.
func normalizeMetricName(name string) string {
	return strings.TrimSuffix(name, "_total")
}

func (c *Check) submitMetrics(s sender.Sender, families []prometheus.MetricFamily) {
	for _, mf := range families {
		def, ok := c.metrics[normalizeMetricName(mf.Name)]
		if !ok {
			continue
		}

		for _, sample := range mf.Samples {
			tags := labelsToTags(sample.Metric)
			s.MonotonicCount(metricNs+def.ddName+".count", sample.Value, "", tags)

			if def.coat != nil {
				key := seriesKey(def.ddName, sample.Metric)
				prev := c.prevValues[key]
				delta := sample.Value - prev
				if delta < 0 {
					delta = sample.Value
				}
				c.prevValues[key] = sample.Value

				tagValues := make([]string, len(def.coat.tagKeys))
				for i, k := range def.coat.tagKeys {
					tagValues[i] = sample.Metric[k]
				}
				def.coat.counter.Add(delta, tagValues...)
			}
		}
	}
}

// seriesKey builds a unique key for a metric time series from its name and
// all label values, used to track previous values for delta computation.
func seriesKey(name string, labels prometheus.Metric) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k == "__name__" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		b.WriteByte('|')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

func labelsToTags(m prometheus.Metric) []string {
	tags := make([]string, 0, len(m))
	for k, v := range m {
		if k == "__name__" {
			continue
		}
		tags = append(tags, k+":"+v)
	}
	return tags
}
