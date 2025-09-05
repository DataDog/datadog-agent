// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	metricsInterval = 15 * time.Second
)

// sendAgentCheckMetrics creates and sends metrics series for agent checks
func (c *collectorImpl) sendAgentCheckMetrics(ctx context.Context, timestamp time.Time, agentCheckResults []agentCheckResult) error {
	hostname, _ := c.hostname.Get(ctx)
	ts := float64(timestamp.Unix())

	if len(agentCheckResults) == 0 {
		log.Debugf("No agent checks found")
		return nil
	}

	for _, check := range agentCheckResults {
		status := "unknown"
		switch check.status {
		case "OK":
			status = "healthy"
		case "WARNING":
			status = "warning"
		case "ERROR":
			status = "broken"
		}

		// Create tags for the check
		tags := []string{
			fmt.Sprintf("integration_type:%v", check.instanceType),
			fmt.Sprintf("integration_name:%v", check.instanceName),
			fmt.Sprintf("status:%s", status),
		}

		log.Debugf("Sending agent check metric: %s = %+v", "datadog.agent.integration.status", tags)

		// Create individual check status metric
		aggregator.AddRecurrentSeries(&metrics.Serie{
			Name:   "datadog.agent.integration.status",
			Points: []metrics.Point{{Value: 1.0, Ts: ts}},
			Tags:   tagset.CompositeTagsFromSlice(tags),
			Host:   hostname,
			MType:  metrics.APIGaugeType,
		})
	}

	return nil
}

func (c *collectorImpl) collectCheckMetrics(ctx context.Context) {
	agentChecks := c.getAgentCheckResults()

	if err := c.sendAgentCheckMetrics(ctx, time.Now(), agentChecks); err != nil {
		c.log.Errorf("unable to send agent check metrics: %s", err)
	}
}

// startMetricsRunner starts the metrics collection runner
func (c *collectorImpl) startMetricsRunner() {
	if !c.config.GetBool("integration_status_metrics_enabled") {
		return
	}

	if c.metricsRunnerStop != nil {
		return
	}

	c.metricsRunnerStop = make(chan struct{})
	c.metricsRunnerWg.Add(1)

	go func() {
		defer c.metricsRunnerWg.Done()
		ticker := time.NewTicker(metricsInterval)
		defer ticker.Stop()

		c.log.Debug("Agent check metrics runner started")

		for {
			select {
			case <-ticker.C:
				// Collect and send agent check metrics
				c.collectCheckMetrics(context.Background())
			case <-c.metricsRunnerStop:
				c.log.Debug("Agent check metrics runner stopped")
				return
			}
		}
	}()
}

// stopMetricsRunner stops the metrics collection runner
func (c *collectorImpl) stopMetricsRunner() {
	if c.metricsRunnerStop == nil {
		return // not running
	}

	close(c.metricsRunnerStop)
	c.metricsRunnerWg.Wait()
	c.metricsRunnerStop = nil
	c.log.Debug("Agent check metrics runner shutdown complete")
}
