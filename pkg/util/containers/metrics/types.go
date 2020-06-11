// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package metrics

import "time"

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
	// docker.mem.cache
	Cache uint64

	// docker.mem.swap
	Swap        uint64 // See SwapPresent to make sure it's a real zero
	SwapPresent bool

	// docker.mem.rss
	RSS uint64

	RSSHuge      uint64
	MappedFile   uint64
	Pgpgin       uint64
	Pgpgout      uint64
	Pgfault      uint64
	Pgmajfault   uint64
	InactiveAnon uint64
	ActiveAnon   uint64
	InactiveFile uint64
	ActiveFile   uint64
	Unevictable  uint64

	// docker.mem.limit
	// Note: docker.mem.in_use = docker.mem.rss / docker.mem.limit
	HierarchicalMemoryLimit uint64

	// docker.mem.sw_limit
	// Note: docker.mem.sw_in_use = docker.mem.rss / docker.mem.sw_limit
	HierarchicalMemSWLimit uint64 // One can safely assume 0 == absent

	TotalCache        uint64
	TotalRSS          uint64
	TotalRSSHuge      uint64
	TotalMappedFile   uint64
	TotalPgpgIn       uint64
	TotalPgpgOut      uint64
	TotalPgFault      uint64
	TotalPgMajFault   uint64
	TotalInactiveAnon uint64
	TotalActiveAnon   uint64
	TotalInactiveFile uint64
	TotalActiveFile   uint64
	TotalUnevictable  uint64
	MemUsageInBytes   uint64

	// docker.mem.soft_limit
	SoftMemLimit uint64

	// docker.kmem.usage
	KernMemUsage uint64

	// docker.mem.failed_count
	MemFailCnt uint64

	// docker.mem.private_working_set
	PrivateWorkingSet uint64

	// docker.mem.commit_bytes
	CommitBytes uint64

	// docker.mem.commit_peak_bytes
	CommitPeakBytes uint64
}

// ContainerCPUStats stores CPU times for a cgroup.
// Unit is userspace scheduling unit (USER_HZ, usually 1/100)
type ContainerCPUStats struct {
	Timestsamp time.Time

	// docker.cpu.system
	System uint64

	// docker.cpu.user
	User uint64

	// docker.cpu.usage
	UsageTotal float64

	SystemUsage uint64

	// docker.cpu.shares
	Shares uint64

	// docker.cpu.throttled
	NrThrottled uint64
	NrPeriod    uint64

	// docker.thread.count
	ThreadCount uint64
}

// ContainerIOStats store I/O statistics about a cgroup.
// Sums are stored in ReadBytes and WriteBytes
type ContainerIOStats struct {
	// docker.io.read_bytes
	ReadBytes uint64

	// docker.io.write_bytes
	WriteBytes uint64

	// docker.io.read_bytes
	DeviceReadBytes map[string]uint64

	// docker.io.write_bytes
	DeviceWriteBytes map[string]uint64

	// docker.container.open_fds
	OpenFiles uint64
}

// ContainerMetrics wraps all container metrics
type ContainerMetrics struct {
	CPU    *ContainerCPUStats
	Memory *ContainerMemStats
	IO     *ContainerIOStats
}

// ContainerLimits represents the (normally static) resources limits set when a container is created
type ContainerLimits struct {
	CPUPeriodQuotaHz float64
	CPULimit         float64
	MemLimit         uint64
	ThreadLimit      uint64
}

// ContainerMetricsProvider defines the API for any implementation that could provide container metrics
type ContainerMetricsProvider interface {
	GetContainerMetrics(containerID string) (*ContainerMetrics, error)
	GetContainerLimits(containerID string) (*ContainerLimits, error)
	GetNetworkMetrics(containerID string, networks map[string]string) (ContainerNetStats, error)
}
