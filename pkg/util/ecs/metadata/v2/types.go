// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package v2

// Task represents a task as returned by the ECS metadata API v2.
type Task struct {
	ClusterName           string             `json:"Cluster"`
	Containers            []Container        `json:"Containers"`
	KnownStatus           string             `json:"KnownStatus"`
	TaskARN               string             `json:"TaskARN"`
	Family                string             `json:"Family"`
	Version               string             `json:"Revision"`
	Limits                map[string]float64 `json:"Limits"`
	DesiredStatus         string             `json:"DesiredStatus"`
	AvailabilityZone      string             `json:"AvailabilityZone"`
	ContainerInstanceTags map[string]string  `json:"ContainerInstanceTags,omitempty"` // undocumented
	TaskTags              map[string]string  `json:"TaskTags,omitempty"`              // undocumented
}

// Container represents a container within a task.
type Container struct {
	Name          string            `json:"Name"`
	Limits        map[string]uint64 `json:"Limits"`
	ImageID       string            `json:"ImageID,omitempty"`
	StartedAt     string            `json:"StartedAt"` // 2017-11-17T17:14:07.781711848Z
	DockerName    string            `json:"DockerName"`
	Type          string            `json:"Type"`
	Image         string            `json:"Image"`
	Labels        map[string]string `json:"Labels"`
	KnownStatus   string            `json:"KnownStatus"`
	DesiredStatus string            `json:"DesiredStatus"`
	DockerID      string            `json:"DockerID"`
	CreatedAt     string            `json:"CreatedAt"`
	Networks      []Network         `json:"Networks"`
	Ports         []Port            `json:"Ports"`
}

// Network represents the network of a container
type Network struct {
	NetworkMode   string   `json:"NetworkMode"`   // as of today the only supported mode is awsvpc
	IPv4Addresses []string `json:"IPv4Addresses"` // one-element list
}

// Port represents the ports of a container
type Port struct {
	ContainerPort uint16 `json:"ContainerPort,omitempty"`
	Protocol      string `json:"Protocol,omitempty"`
	HostPort      uint16 `json:"HostPort,omitempty"`
}

// ContainerStats represents the statistics of a container as returned by the
// ECS metadata API v2.
type ContainerStats struct {
	CPU      CPUStats    `json:"cpu_stats"`
	Memory   MemStats    `json:"memory_stats"`
	IO       IOStats     `json:"blkio_stats"`
	Networks NetStatsMap `json:"networks"`
	//Pids    []int32  `json:"pids_stats"` // seems to be always empty
}

// NetStatsMap represents a map of networks stats
type NetStatsMap map[string]NetStats

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
	Details  DetailedMem `json:"stats"`
	Limit    uint64      `json:"limit"`
	MaxUsage uint64      `json:"max_usage"`
	Usage    uint64      `json:"usage"`
}

// DetailedMem stores detailed stats about memory usage
type DetailedMem struct {
	RSS     uint64 `json:"rss"`
	Cache   uint64 `json:"cache"`
	PgFault uint64 `json:"pgfault"`
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
