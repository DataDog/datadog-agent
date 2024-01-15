// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package metrics registers all the different collectors for container-related
// metrics.
package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"

	// Register all the collectors
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/containerd"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/cri"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/docker"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/ecsfargate"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/kubelet"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/system"
)

// Runtime is a typed string with all supported runtimes
type Runtime = provider.Runtime

// Collector defines an interface allowing to get stats from a containerID.
type Collector = provider.Collector

// CollectorMetadata contains the characteristics of a collector to be registered with RegisterCollector
type CollectorMetadata = provider.CollectorMetadata

// Provider interface allows to mock the metrics provider
type Provider = provider.Provider

// ContainerMemStats stores memory statistics.
type ContainerMemStats = provider.ContainerMemStats

// ContainerCPUStats stores CPU stats.
type ContainerCPUStats = provider.ContainerCPUStats

// DeviceIOStats stores Device IO stats.
type DeviceIOStats = provider.DeviceIOStats

// ContainerIOStats store I/O statistics about a container.
type ContainerIOStats = provider.ContainerIOStats

// ContainerPIDStats stores stats about threads & processes.
type ContainerPIDStats = provider.ContainerPIDStats

// InterfaceNetStats stores network statistics about a network interface
type InterfaceNetStats = provider.InterfaceNetStats

// ContainerNetworkStats stores network statistics about a container per interface
type ContainerNetworkStats = provider.ContainerNetworkStats

// ContainerStats wraps all container metrics
type ContainerStats = provider.ContainerStats

// GetProvider returns the metrics provider singleton
var GetProvider = provider.GetProvider
