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
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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

type metricKind int

const (
	counterMetric metricKind = iota
	gaugeMetric
)

// coatMetric associates a COAT telemetry metric with the Prometheus label names
// it expects, in the order required by the telemetry component.
type coatMetric struct {
	counter telemetry.Counter
	gauge   telemetry.Gauge
	kind    metricKind
	tagKeys []string
}

// metricDef defines how a Prometheus metric is mapped to a Datadog metric name
// and optionally to a COAT telemetry counter.
type metricDef struct {
	ddName string
	kind   metricKind
	coat   *coatMetric
}

type sharedState struct {
	prevValues map[string]float64
	gaugeKeys  map[string]gaugeSeries
	mu         sync.Mutex
}

type gaugeSeries struct {
	coat      *coatMetric
	tagValues []string
}

func buildMetricDefs(tm telemetry.Component) map[string]metricDef {
	return map[string]metricDef{
		"datadog_csi_driver_node_publish_volume_attempts": {
			ddName: "node_publish_volume_attempts",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "node_publish_volume_attempts", []string{"status", "type"}, "CSI node publish volume attempts"),
				kind:    counterMetric,
				tagKeys: []string{"status", "type"},
			},
		},
		"datadog_csi_driver_node_unpublish_volume_attempts": {
			ddName: "node_unpublish_volume_attempts",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "node_unpublish_volume_attempts", []string{"status"}, "CSI node unpublish volume attempts"),
				kind:    counterMetric,
				tagKeys: []string{"status"},
			},
		},
		"datadog_csi_driver_library_resolutions": {
			ddName: "library_resolutions",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "library_resolutions", []string{"library", "result"}, "CSI driver library resolution attempts"),
				kind:    counterMetric,
				tagKeys: []string{"library", "result"},
			},
		},
		"datadog_csi_driver_library_cleanup": {
			ddName: "library_cleanup",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "library_cleanup", []string{"library", "status", "strategy"}, "CSI driver library cleanup attempts"),
				kind:    counterMetric,
				tagKeys: []string{"library", "status", "strategy"},
			},
		},
		"datadog_csi_driver_libraries_cached": {
			ddName: "libraries_cached",
			kind:   gaugeMetric,
			coat: &coatMetric{
				gauge:   tm.NewGauge(CheckName, "libraries_cached", []string{"library"}, "CSI driver cached library versions"),
				kind:    gaugeMetric,
				tagKeys: []string{"library"},
			},
		},
		"datadog_csi_driver_libraries_cached_bytes": {
			ddName: "libraries_cached_bytes",
			kind:   gaugeMetric,
			coat: &coatMetric{
				gauge:   tm.NewGauge(CheckName, "libraries_cached_bytes", []string{"library"}, "CSI driver cached library bytes"),
				kind:    gaugeMetric,
				tagKeys: []string{"library"},
			},
		},
		"datadog_csi_driver_library_volume_links": {
			ddName: "library_volume_links",
			kind:   gaugeMetric,
			coat: &coatMetric{
				gauge:   tm.NewGauge(CheckName, "library_volume_links", []string{"library"}, "CSI driver library volume links"),
				kind:    gaugeMetric,
				tagKeys: []string{"library"},
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
	state      *sharedState
}

// Factory creates a new check factory.
func Factory(tm telemetry.Component) option.Option[func() check.Check] {
	metricDefs := buildMetricDefs(tm)
	state := &sharedState{
		prevValues: make(map[string]float64),
		gaugeKeys:  make(map[string]gaugeSeries),
	}
	return option.New(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBase(CheckName),
			metrics:   metricDefs,
			state:     state,
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
		c.deleteEndpointGaugeSeries()
		s.ServiceCheck(metricNs+"openmetrics.health", servicecheck.ServiceCheckCritical, "", nil, fmt.Sprintf("endpoint unreachable: %s", err))
		return fmt.Errorf("scrape %s: %w", c.config.OpenmetricsEndpoint, err)
	}

	s.ServiceCheck(metricNs+"openmetrics.health", servicecheck.ServiceCheckOK, "", nil, "")

	c.submitMetrics(s, metrics)

	return nil
}

// Cancel clears long-lived COAT gauge series when the check is unscheduled.
func (c *Check) Cancel() {
	c.deleteEndpointGaugeSeries()
	c.CheckBase.Cancel()
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
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	currentGaugeKeys := make(map[string]struct{})

	for _, mf := range families {
		def, ok := c.metrics[normalizeMetricName(mf.Name)]
		if !ok {
			continue
		}

		for _, sample := range mf.Samples {
			tags := labelsToTags(sample.Metric)
			switch def.kind {
			case counterMetric:
				s.MonotonicCount(metricNs+def.ddName+".count", sample.Value, "", tags)
			case gaugeMetric:
				s.Gauge(metricNs+def.ddName, sample.Value, "", tags)
			}

			if def.coat != nil {
				tagValues := make([]string, len(def.coat.tagKeys))
				for i, k := range def.coat.tagKeys {
					tagValues[i] = sample.Metric[k]
				}

				switch def.coat.kind {
				case counterMetric:
					key := c.endpointSeriesKey(def.ddName, sample.Metric)
					prev := c.state.prevValues[key]
					delta := sample.Value - prev
					if delta < 0 {
						delta = sample.Value
					}
					c.state.prevValues[key] = sample.Value
					def.coat.counter.Add(delta, tagValues...)
				case gaugeMetric:
					def.coat.gauge.Set(sample.Value, tagValues...)
					key := c.endpointSeriesKey(def.ddName, sample.Metric)
					currentGaugeKeys[key] = struct{}{}
					c.state.gaugeKeys[key] = gaugeSeries{
						coat:      def.coat,
						tagValues: slicesClone(tagValues),
					}
				}
			}
		}
	}

	c.deleteStaleEndpointGaugeSeriesLocked(currentGaugeKeys)
}

func (c *Check) deleteEndpointGaugeSeries() {
	c.deleteStaleEndpointGaugeSeries(nil)
}

func (c *Check) deleteStaleEndpointGaugeSeries(currentGaugeKeys map[string]struct{}) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	c.deleteStaleEndpointGaugeSeriesLocked(currentGaugeKeys)
}

func (c *Check) deleteStaleEndpointGaugeSeriesLocked(currentGaugeKeys map[string]struct{}) {
	for key, prev := range c.state.gaugeKeys {
		if _, ok := currentGaugeKeys[key]; ok {
			continue
		}
		if !strings.HasPrefix(key, c.config.OpenmetricsEndpoint+"|") {
			continue
		}
		prev.coat.gauge.Delete(prev.tagValues...)
		delete(c.state.gaugeKeys, key)
	}
}

func (c *Check) endpointSeriesKey(name string, labels prometheus.Metric) string {
	return c.config.OpenmetricsEndpoint + "|" + seriesKey(name, labels)
}

func slicesClone(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
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
