// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

// CgroupMemStat stores memory statistics about a cgroup.
type CgroupMemStat struct {
	ContainerID             string
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
}

// CgroupTimesStat stores CPU times for a cgroup.
// Unit is userspace scheduling unit (USER_HZ, usually 1/100)
type CgroupTimesStat struct {
	ContainerID string
	System      uint64
	User        uint64
	UsageTotal  float64
	SystemUsage uint64
	Shares      uint64
}

// CgroupIOStat store I/O statistics about a cgroup.
// Sums are stored in ReadBytes and WriteBytes
type CgroupIOStat struct {
	ContainerID      string
	ReadBytes        uint64
	WriteBytes       uint64
	DeviceReadBytes  map[string]uint64
	DeviceWriteBytes map[string]uint64
}

// ContainerCgroup is a structure that stores paths and mounts for a cgroup.
// It provides several methods for collecting stats about the cgroup using the
// paths and mounts metadata.
type ContainerCgroup struct {
	ContainerID string
	Pids        []int32
	Paths       map[string]string
	Mounts      map[string]string
}

// NetworkDestination holds one network destination subnet and it's linked interface name
type NetworkDestination struct {
	Interface string
	Subnet    uint64
	Mask      uint64
}
