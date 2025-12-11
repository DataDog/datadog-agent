// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

// ProcessParams defines filtering parameters for process queries
type ProcessParams struct {
	PIDs             []int32  `json:"pids,omitempty"`
	ProcessNames     []string `json:"process_names,omitempty"`
	RegexFilter      string   `json:"regex_filter,omitempty"`
	IncludeStats     bool     `json:"include_stats"`
	IncludeIO        bool     `json:"include_io"`
	IncludeNet       bool     `json:"include_net"`
	Limit            int      `json:"limit,omitempty"`
	SortBy           string   `json:"sort_by,omitempty"` // cpu, memory, pid, name
	Ascending        bool     `json:"ascending"`
	Compact          bool     `json:"compact"`            // Return minimal fields (pid, name, cpu%, mem%)
	MaxCmdLineLength int      `json:"max_cmd_length"`     // Truncate command line to this length (0 = no truncation)
}

// ProcessSnapshot represents a point-in-time view of processes
type ProcessSnapshot struct {
	Timestamp        int64                 `json:"ts"`
	HostInfo         *HostMetadata         `json:"host,omitempty"`
	Processes        []*ProcessInfo        `json:"procs,omitempty"`
	CompactProcesses []*CompactProcessInfo `json:"ps,omitempty"` // Used in compact mode
	TotalCount       int                   `json:"total"`
	FilteredCount    int                   `json:"count"`
}

// ProcessInfo contains detailed process information
type ProcessInfo struct {
	// Basic information
	PID         int32    `json:"pid"`
	PPID        int32    `json:"ppid,omitempty"`
	Name        string   `json:"name"`
	Executable  string   `json:"exe,omitempty"`
	CommandLine []string `json:"cmd,omitempty"`
	Username    string   `json:"user,omitempty"`
	UserID      int32    `json:"uid,omitempty"`
	GroupID     int32    `json:"gid,omitempty"`

	// Timing
	CreateTime int64  `json:"ctime,omitempty"`
	Status     string `json:"status,omitempty"` // R, S, D, Z, T, W

	// Resource usage
	CPUPercent float64     `json:"cpu,omitempty"`
	CPUTime    *CPUTime    `json:"cpu_time,omitempty"`
	Memory     *MemoryInfo `json:"mem,omitempty"`
	IOCounters *IOInfo     `json:"io,omitempty"`

	// File descriptors
	OpenFiles  int32 `json:"open_fds,omitempty"`
	NumThreads int32 `json:"threads,omitempty"`

	// Network (if available)
	TCPPorts    []uint16 `json:"tcp,omitempty"`
	UDPPorts    []uint16 `json:"udp,omitempty"`
	Connections int32    `json:"conns,omitempty"`

	// Service discovery
	Service *ServiceInfo `json:"svc,omitempty"`

	// Container association
	ContainerID string `json:"cid,omitempty"`

	// APM injection status
	APMInjected bool `json:"apm,omitempty"`
}

// CompactProcessInfo is a minimal representation for compact mode
type CompactProcessInfo struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu,omitempty"`
	MemPercent float32 `json:"mem,omitempty"`
	MemRSS     uint64  `json:"rss,omitempty"`
	Cmd        string  `json:"cmd,omitempty"` // Single truncated string instead of []string
}

// MemoryInfo contains memory usage information
type MemoryInfo struct {
	RSS     uint64  `json:"rss"`     // Resident Set Size in bytes
	VMS     uint64  `json:"vms"`     // Virtual Memory Size in bytes
	Shared  uint64  `json:"shared"`  // Shared memory in bytes
	Text    uint64  `json:"text"`    // Text segment size
	Data    uint64  `json:"data"`    // Data segment size
	Percent float32 `json:"percent"` // Memory usage percentage
}

// CPUTime contains CPU time information
type CPUTime struct {
	User   float64 `json:"user"`   // User CPU time in seconds
	System float64 `json:"system"` // System CPU time in seconds
	Idle   float64 `json:"idle"`   // Idle time
}

// IOInfo contains I/O statistics
type IOInfo struct {
	ReadCount  uint64  `json:"read_count"`
	WriteCount uint64  `json:"write_count"`
	ReadBytes  uint64  `json:"read_bytes"`
	WriteBytes uint64  `json:"write_bytes"`
	ReadRate   float64 `json:"read_rate"`  // bytes/sec
	WriteRate  float64 `json:"write_rate"` // bytes/sec
}

// ServiceInfo contains service discovery information
type ServiceInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	DDService   string   `json:"dd_service,omitempty"`
	Language    string   `json:"language,omitempty"`
	Type        string   `json:"type,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// HostMetadata contains host-level information
type HostMetadata struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Platform      string `json:"platform"`
	KernelVersion string `json:"kernel_version"`
	CPUCores      int    `json:"cpu_cores"`
	TotalMemory   uint64 `json:"total_memory"`
}
