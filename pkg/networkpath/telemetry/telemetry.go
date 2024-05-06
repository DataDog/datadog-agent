// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package telemetry handles Network Path telemetry
package telemetry

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// SubmitNetworkPathTelemetry submits Network Path related telemetry
func SubmitNetworkPathTelemetry(sender metricsender.MetricSender, path payload.NetworkPath, checkDuration time.Duration, checkInterval time.Duration, tags []string) {
	newTags := utils.CopyStrings(tags)

	sender.Gauge("datadog.network_path.check_duration", checkDuration.Seconds(), newTags)

	if checkInterval > 0 {
		sender.Gauge("datadog.network_path.check_interval", checkInterval.Seconds(), newTags)
	}

	sender.Gauge("datadog.network_path.path.monitored", float64(1), newTags)
	if len(path.Hops) > 0 {
		lastHop := path.Hops[len(path.Hops)-1]
		if lastHop.Success {
			sender.Gauge("datadog.network_path.path.hops", float64(len(path.Hops)), newTags)
		}
		sender.Gauge("datadog.network_path.path.reachable", float64(utils.BoolToFloat64(lastHop.Success)), newTags)
		sender.Gauge("datadog.network_path.path.unreachable", float64(utils.BoolToFloat64(!lastHop.Success)), newTags)
	}
}
