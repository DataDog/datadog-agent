// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

type autoscalingType string

const (
	aliveTelemetryInterval = 5 * time.Minute
)

func StartLocalTelemetry(ctx context.Context, s sender.Sender, t autoscalingType, tags []string) {
	submit := func() {
		metricName := fmt.Sprintf("datadog.cluster_agent.autoscaling.%s.running", t)
		s.Gauge(metricName, 1, "", tags)
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
