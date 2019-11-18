// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2019 Datadog, Inc.

package ecs

import "github.com/DataDog/datadog-agent/pkg/util/retry"

// ContainerStats represents the stats payload for a container
// reported by the ecs stats api.
type ContainerStats struct {
	CPU     CPUStats `json:"cpu_stats"`
	Memory  MemStats `json:"memory_stats"`
	IO      IOStats  `json:"blkio_stats"`
	Network NetStats `json:"network"`
	// Pids    []int32  `json:"pids_stats"` // seems to be always empty
}

// CPUStats represents an ECS container CPU usage
type CPUStats struct {
	Usage  CPUUsage `json:"cpu_usage"`
	System uint64   `json:"system_cpu_usage"`
}

// CPUUsage represents the details of ECS container CPU usage
type CPUUsage struct {
	Total      uint64 `json:"total_usage"`
	Usermode   uint64 `json:"usage_in_usermode"`
	Kernelmode uint64 `json:"usage_in_kernelmode"`
}

// MemStats represents an ECS container memory usage
type MemStats struct {
	Details DetailedMem `json:"stats"`
}

// DetailedMem stores detailed stats about memory usage
type DetailedMem struct {
	RSS      uint64 `json:"rss"`
	Cache    uint64 `json:"cache"`
	Usage    uint64 `json:"usage"`
	MaxUsage uint64 `json:"max_usage"`
	Limit    uint64 `json:"hierarchical_memory_limit"`
	PgFault  uint64 `json:"pgfault"`
}

// IOStats represents an ECS container IO throughput
type IOStats struct {
	BytesPerDeviceAndKind []OPStat `json:"io_service_bytes_recursive"`
	OPPerDeviceAndKind    []OPStat `json:"io_serviced_recursive"`
	ReadBytes             uint64   // calculated by aggregating OPStats
	WriteBytes            uint64   // calculated by aggregating OPStats
}

// OPStat stores a value (amount of op or bytes) for a kind of operation and a specific block device.
type OPStat struct {
	Major int64  `json:"major"`
	Minor int64  `json:"minor"`
	Kind  string `json:"op"`
	Value uint64 `json:"value"`
}

// NetStats represents an ECS container network usage
type NetStats struct {
	RxBytes   uint64 `json:"rx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	TxBytes   uint64 `json:"tx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
}

// Util wraps interactions with the ECS agent
type Util struct {
	// used to setup the ECSUtil
	initRetry retry.Retrier
	agentURL  string
}
