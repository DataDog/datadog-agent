// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"context"
	"os"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// ContainersRunningTelemetry reports environment information (e.g containers running) when the runtime security component is running
type ContainersRunningTelemetry struct {
	cfg        *config.RuntimeSecurityConfig
	containers *ContainersTelemetry
}

func NewContainersRunningTelemetry(cfg *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, wmeta workloadmeta.Component) (*ContainersRunningTelemetry, error) {
	telemetrySender := NewSimpleTelemetrySenderFromStatsd(statsdClient)
	containersTelemetry, err := NewContainersTelemetry(telemetrySender, wmeta)
	if err != nil {
		return nil, err
	}

	return &ContainersRunningTelemetry{
		cfg:        cfg,
		containers: containersTelemetry,
	}, nil
}

func (t *ContainersRunningTelemetry) Run(ctx context.Context) {
	log.Info("started collecting Runtime Security Agent telemetry")
	defer log.Info("stopping Runtime Security Agent telemetry")

	metricsTicker := time.NewTicker(1 * time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			if err := t.reportContainers(); err != nil {
				log.Debugf("couldn't report containers: %v", err)
			}
		}
	}
}

func (t *ContainersRunningTelemetry) reportContainers() error {
	var fargate bool
	if os.Getenv("ECS_FARGATE") == "true" || os.Getenv("DD_ECS_FARGATE") == "true" || os.Getenv("DD_EKS_FARGATE") == "true" {
		fargate = true
	}

	var metricName string
	if t.cfg.RuntimeEnabled {
		metricName = metrics.MetricSecurityAgentRuntimeContainersRunning
		if fargate {
			metricName = metrics.MetricSecurityAgentFargateRuntimeContainersRunning
		}
	} else if t.cfg.FIMEnabled {
		metricName = metrics.MetricSecurityAgentFIMContainersRunning
		if fargate {
			metricName = metrics.MetricSecurityAgentFargateFIMContainersRunning
		}
	} else {
		// nothing to report
		return nil
	}

	t.containers.ReportContainers(metricName)

	return nil
}
