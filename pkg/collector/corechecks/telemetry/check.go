// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package telemetry

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	dto "github.com/prometheus/client_model/go"
)

const (
	checkName = "telemetry"
)

func init() {
	core.RegisterCheck(checkName, CheckFactory)
}

// Check reports container images
type Check struct {
	core.CheckBase
	sender aggregator.Sender
	stopCh chan struct{}
}

// CheckFactory registers the container_image check
func CheckFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(checkName),
		stopCh:    make(chan struct{}),
	}
}

// Configure parses the check configuration and initializes the telemetry check
func (c *Check) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(integrationConfigDigest, initConfig, config, source); err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	c.sender = sender

	return nil
}

// Run starts the telemetry check
func (c *Check) Run() error {
	log.Infof("Starting telemetry check %q", c.ID())
	defer log.Infof("Finished telemetry check %q", c.ID())

	registry := telemetry.GetRegistry()

	metricsMap, err := registry.Gather()
	if err != nil {
		return err
	}

	for _, mf := range metricsMap {
		for _, metric := range mf.GetMetric() {
			name := mf.GetName()

			labels := metric.GetLabel()
			tags := make([]string, len(labels))
			for _, label := range labels {
				tags = append(tags, fmt.Sprintf("%s:%s", label.GetName(), label.GetValue()))
			}

			switch mf.GetType() {
			case dto.MetricType_GAUGE:
				val := metric.GetGauge().GetValue()
				c.sender.Gauge(name, val, "HOST", tags)
			case dto.MetricType_COUNTER:
				val := metric.GetCounter().GetValue()
				c.sender.Counter(name, val, "HOST", tags)
			case dto.MetricType_HISTOGRAM:
				lower := 0.0
				// assuming here that buckets are in ascending order
				for _, b := range metric.GetHistogram().Bucket {
					upper := b.GetUpperBound()
					count := b.GetCumulativeCount()
					c.sender.HistogramBucket(name, int64(count), lower, upper, true, "HOST", tags, false)
					lower = upper
				}
			}
		}
	}

	c.sender.Commit()

	return nil
}
