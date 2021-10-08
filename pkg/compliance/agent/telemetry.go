// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/containers/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	containersCountMetricName = "datadog.security_agent.compliance.containers_running"
)

// telemetry reports environment information (e.g containers running) when the compliance component is running
type telemetry struct {
	sender   aggregator.Sender
	detector collectors.DetectorInterface
}

func newTelemetry() (*telemetry, error) {
	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		return nil, err
	}

	return &telemetry{
		sender:   sender,
		detector: collectors.NewDetector(""),
	}, nil
}

func (t *telemetry) run(ctx context.Context) {
	log.Info("Start collecting Compliance telemetry")
	defer log.Info("Stopping Compliance telemetry")

	metricsTicker := time.NewTicker(2 * time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			if err := t.reportContainers(); err != nil {
				log.Debugf("Couldn't report containers: %v", err)
			}
		}
	}
}

func (t *telemetry) reportContainers() error {
	collector, _, err := t.detector.GetPreferred()
	if err != nil {
		return err
	}

	containers, err := collector.List()
	if err != nil {
		return err
	}

	for _, container := range containers {
		t.sender.Gauge(containersCountMetricName, 1.0, "", []string{"container_id:" + container.ID})
	}

	t.sender.Commit()

	return nil
}
