// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"context"
	"errors"
	"os"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	sectelemetry "github.com/DataDog/datadog-agent/pkg/security/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// telemetry reports environment information (e.g containers running) when the runtime security component is running
type telemetry struct {
	containers            *sectelemetry.ContainersTelemetry
	runtimeSecurityClient *RuntimeSecurityClient
}

func newTelemetry(statsdClient statsd.ClientInterface, wmeta workloadmeta.Component) (*telemetry, error) {
	runtimeSecurityClient, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	telemetrySender := sectelemetry.NewSimpleTelemetrySenderFromStatsd(statsdClient)
	containersTelemetry, err := sectelemetry.NewContainersTelemetry(telemetrySender, wmeta)
	if err != nil {
		return nil, err
	}

	return &telemetry{
		containers:            containersTelemetry,
		runtimeSecurityClient: runtimeSecurityClient,
	}, nil
}

func (t *telemetry) run(ctx context.Context) {
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

func (t *telemetry) fetchConfig() (*api.SecurityConfigMessage, error) {
	cfg, err := t.runtimeSecurityClient.GetConfig()
	if err != nil {
		return cfg, errors.New("couldn't fetch config from runtime security module")
	}
	return cfg, nil
}

func (t *telemetry) reportContainers() error {
	// retrieve the runtime security module config
	cfg, err := t.fetchConfig()
	if err != nil {
		return err
	}

	var fargate bool
	if os.Getenv("ECS_FARGATE") == "true" || os.Getenv("DD_ECS_FARGATE") == "true" || os.Getenv("DD_EKS_FARGATE") == "true" {
		fargate = true
	}

	var metricName string
	if cfg.RuntimeEnabled {
		metricName = metrics.MetricSecurityAgentRuntimeContainersRunning
		if fargate {
			metricName = metrics.MetricSecurityAgentFargateRuntimeContainersRunning
		}
	} else if cfg.FIMEnabled {
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
