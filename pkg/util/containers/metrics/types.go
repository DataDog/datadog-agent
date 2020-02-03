// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package metrics

// InterfaceNetStats stores network statistics about a Docker network interface
type InterfaceNetStats struct {
	NetworkName string
	BytesSent   uint64
	BytesRcvd   uint64
	PacketsSent uint64
	PacketsRcvd uint64
}

// ContainerNetStats stores network statistics about a Docker container per interface
type ContainerNetStats []*InterfaceNetStats

// ContainerMemStats stores memory statistics about a cgroup.
type ContainerMemStats struct {
	Cache                   uint64
	Swap                    uint64 // See SwapPresent to make sure it's a real zero
	SwapPresent             bool
	RSS                     uint64
	RSSHuge                 uint64
	MappedFile              uint64
	Pgpgin                  uint64
	Pgpgout                 uint64
	Pgfault                 uint64
	Pgmajfault              uint64
	InactiveAnon            uint64
	ActiveAnon              uint64
	InactiveFile            uint64
	ActiveFile              uint64
	Unevictable             uint64
	HierarchicalMemoryLimit uint64
	HierarchicalMemSWLimit  uint64 // One can safely assume 0 == absent
	TotalCache              uint64
	TotalRSS                uint64
	TotalRSSHuge            uint64
	TotalMappedFile         uint64
	TotalPgpgIn             uint64
	TotalPgpgOut            uint64
	TotalPgFault            uint64
	TotalPgMajFault         uint64
	TotalInactiveAnon       uint64
	TotalActiveAnon         uint64
	TotalInactiveFile       uint64
	TotalActiveFile         uint64
	TotalUnevictable        uint64
	MemUsageInBytes         uint64
	SoftMemLimit            uint64
	KernMemUsage            uint64
	MemFailCnt              uint64
}

// ContainerCPUStats stores CPU times for a cgroup.
// Unit is userspace scheduling unit (USER_HZ, usually 1/100)
type ContainerCPUStats struct {
	System      uint64
	User        uint64
	UsageTotal  float64
	SystemUsage uint64
	Shares      uint64
	NrThrottled uint64
	ThreadCount uint64
}

// ContainerIOStats store I/O statistics about a cgroup.
// Sums are stored in ReadBytes and WriteBytes
type ContainerIOStats struct {
	ReadBytes        uint64
	WriteBytes       uint64
	DeviceReadBytes  map[string]uint64
	DeviceWriteBytes map[string]uint64
	OpenFiles        uint64
}

// ContainerMetrics wraps all container metrics
type ContainerMetrics struct {
	CPU    *ContainerCPUStats
	Memory *ContainerMemStats
	IO     *ContainerIOStats
}

// ContainerLimits represents the (normally static) resources limits set when a container is created
type ContainerLimits struct {
	CPULimit    float64
	MemLimit    uint64
	ThreadLimit uint64
}

// ContainerMetricsProvider defines the API for any implementation that could provide container metrics
type ContainerMetricsProvider interface {
	GetContainerMetrics(containerID string) (*ContainerMetrics, error)
	GetContainerLimits(containerID string) (*ContainerLimits, error)
	GetNetworkMetrics(containerID string, networks map[string]string) (ContainerNetStats, error)
}
