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
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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

// metricDef defines how a Prometheus metric is mapped to a Datadog metric name.
type metricDef struct {
	ddName string
}

var metricDefs = map[string]metricDef{
	"datadog_csi_driver_node_publish_volume_attempts": {
		ddName: "node_publish_volume_attempts",
	},
	"datadog_csi_driver_node_unpublish_volume_attempts": {
		ddName: "node_unpublish_volume_attempts",
	},
}

// Check collects CSI driver metrics from its Prometheus endpoint.
type Check struct {
	core.CheckBase
	config     csiDriverConfig
	httpClient http.Client
	metrics    map[string]metricDef
}

// Factory creates a new check factory.
func Factory() option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBase(CheckName),
			metrics:   metricDefs,
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
		}
	}
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
