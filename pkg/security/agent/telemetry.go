// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/pkg/errors"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// telemetry reports environment information (e.g containers running) when the runtime security component is running
type telemetry struct {
	detector              collectors.DetectorInterface
	runtimeSecurityClient *RuntimeSecurityClient
	statsdClient          *statsd.Client
}

func newTelemetry() (*telemetry, error) {
	runtimeSecurityClient, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	var statsdClient *statsd.Client
	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		statsdAddr = fmt.Sprintf("%s:%d", coreconfig.GetBindHost(), coreconfig.Datadog.GetInt("dogstatsd_port"))
	}

	if statsdClient, err = statsd.New(statsdAddr); err != nil {
		return nil, err
	}

	return &telemetry{
		detector:              collectors.NewDetector(""),
		runtimeSecurityClient: runtimeSecurityClient,
		statsdClient:          statsdClient,
	}, nil
}

func (t *telemetry) run(ctx context.Context) {
	log.Info("started collecting Runtime Security Agent telemetry")
	defer log.Info("stopping Runtime Security Agent telemetry")

	metricsTicker := time.NewTicker(5 * time.Minute)
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

func (t *telemetry) reportContainers() error {
	// retrieve the runtime security module config
	cfg, err := t.runtimeSecurityClient.GetConfig()
	if err != nil {
		return errors.Errorf("couldn't fetch config from runtime security module")
	}

	var metricName string
	if cfg.RuntimeEnabled {
		metricName = metrics.MetricSecurityAgentRuntimeContainersRunning
	} else if cfg.FIMEnabled {
		metricName = metrics.MetricSecurityAgentFIMContainersRunning
	} else {
		// nothing to report
		return nil
	}

	collector, _, err := t.detector.GetPreferred()
	if err != nil {
		return err
	}

	containers, err := collector.List()
	if err != nil {
		return err
	}

	for _, container := range containers {
		_ = t.statsdClient.Gauge(metricName, 1.0, []string{"container_id:" + container.ID}, 1)
	}

	return nil
}
