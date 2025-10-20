// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
)

const (
	subsystem              = "autoscaling_cluster"
	aliveTelemetryInterval = 5 * time.Minute
)

var autoscalingQueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()

func startLocalTelemetry(ctx context.Context, s sender.Sender, tags []string) {
	submit := func() {
		s.Gauge("datadog.cluster_agent.autoscaling.cluster.running", 1, "", tags)
		s.Commit()
	}

	go func() {
		ticker := time.NewTicker(aliveTelemetryInterval)
		defer ticker.Stop()

		// Submit once immediately and then every ticker
		submit()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				submit()
			}
		}
	}()
}
