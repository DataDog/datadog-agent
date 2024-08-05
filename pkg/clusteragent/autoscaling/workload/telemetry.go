// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	subsystem              = "workload_autoscaling"
	aliveTelemetryInterval = 5 * time.Minute
)

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

// rolloutTriggered tracks the number of patch requests sent by the patcher to the kubernetes api server
var rolloutTriggered = telemetry.NewCounterWithOpts(
	subsystem,
	"rollout_triggered",
	[]string{"owner_kind", "owner_name", "namespace", "status"},
	"Tracks the number of patch requests sent by the patcher to the kubernetes api server",
	commonOpts,
)

func startLocalTelemetry(ctx context.Context, sender sender.Sender, tags []string) {
	submit := func() {
		sender.Gauge("datadog.cluster_agent.autoscaling.workload.running", 1, "", tags)
		sender.Commit()
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
