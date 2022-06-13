// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	containersCountMetricName = "datadog.security_agent.compliance.containers_running"
)

// telemetry reports environment information (e.g containers running) when the compliance component is running
type telemetry struct {
	containers *common.ContainersTelemetry
}

func newTelemetry() (*telemetry, error) {
	containersTelemetry, err := common.NewContainersTelemetry()
	if err != nil {
		return nil, err
	}

	return &telemetry{
		containers: containersTelemetry,
	}, nil
}

func (t *telemetry) run(ctx context.Context) {
	log.Info("Start collecting Compliance telemetry")
	defer log.Info("Stopping Compliance telemetry")

	metricsTicker := time.NewTicker(1 * time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			t.reportContainers()
		}
	}
}

func (t *telemetry) reportContainers() {
	t.containers.ReportContainers(containersCountMetricName)
}
