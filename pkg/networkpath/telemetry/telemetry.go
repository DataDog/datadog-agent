// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package telemetry handles Network Path telemetry
package telemetry

import (
	"sort"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// NetworkPathCollectorType represent the source of the network path data e.g. network_path_integration
type NetworkPathCollectorType string

// CollectorTypeNetworkPathIntegration correspond to the Network Path Integration source type
const CollectorTypeNetworkPathIntegration NetworkPathCollectorType = "network_path_integration"

// CollectorTypeNetworkPathCollector correspond to the Network Path Collector source type
const CollectorTypeNetworkPathCollector NetworkPathCollectorType = "network_path_collector"

// SubmitNetworkPathTelemetry submits Network Path related telemetry
func SubmitNetworkPathTelemetry(sender metricsender.MetricSender, path payload.NetworkPath, pathSource NetworkPathCollectorType, checkDuration time.Duration, checkInterval time.Duration, tags []string) {
	destPortTag := "unspecified"
	if path.Destination.Port > 0 {
		destPortTag = strconv.Itoa(int(path.Destination.Port))
	}
	newTags := append(utils.CopyStrings(tags), []string{
		"collector:" + string(pathSource),
		"protocol:udp", // TODO: Update to protocol from config when we support tcp/icmp
		"destination_hostname:" + path.Destination.Hostname,
		"destination_port:" + destPortTag,
	}...)

	sort.Strings(newTags)

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
