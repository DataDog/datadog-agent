// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package functiontools

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/host"
	"github.com/DataDog/gopsutil/mem"
)

const (
	defaultProcessLimit       = 25
	defaultMaxCmdLineLength   = 200
	defaultMaxProcessSnapshot = 1000
)

func processSnapshot(parameters map[string]string) (*ProcessSnapshot, error) {
	params := parseProcessParams(parameters)

	probe := procutil.NewProcessProbe()
	defer probe.Close()

	scrubber := procutil.NewDefaultDataScrubber()

	now := time.Now()
	procs, err := probe.ProcessesByPID(now, params.IncludeStats)
	if err != nil {
		return nil, err
	}

	totalCount := len(procs)
	processList := make([]*procutil.Process, 0, totalCount)
	for _, proc := range procs {
		processList = append(processList, proc)
	}

	filtered := applyProcessFilters(processList, params)

	if params.ScrubArgs {
		scrubProcesses(scrubber, filtered)
	}

	sortProcesses(filtered, params)

	if params.Limit > 0 && len(filtered) > params.Limit {
		filtered = filtered[:params.Limit]
	}

	return buildProcessSnapshot(filtered, now, totalCount, len(filtered), params), nil
}

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
	Compact          bool     `json:"compact"`        // Return minimal fields (pid, name, cpu%, mem%)
	MaxCmdLineLength int      `json:"max_cmd_length"` // Truncate command line to this length (0 = no truncation)
	ScrubArgs        bool     `json:"-"`              // Internal flag for argument scrubbing
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

// MarshalJSON customizes JSON marshaling for better output
func (p *ProcessSnapshot) MarshalJSON() ([]byte, error) {
	type Alias ProcessSnapshot
	return json.Marshal(&struct {
		*Alias
		TimestampStr string `json:"timestamp_str"`
	}{
		Alias:        (*Alias)(p),
		TimestampStr: time.Unix(p.Timestamp, 0).Format(time.RFC3339),
	})
}

func parseProcessParams(params map[string]string) *ProcessParams {
	result := &ProcessParams{
		IncludeStats:     true,
		Limit:            defaultProcessLimit,
		Compact:          true,
		MaxCmdLineLength: defaultMaxCmdLineLength,
	}

	if value, ok := params["pids"]; ok {
		result.PIDs = parseInt32List(value)
	}

	if value, ok := params["process_names"]; ok {
		result.ProcessNames = parseStringList(value)
	}

	if value, ok := params["regex_filter"]; ok {
		result.RegexFilter = value
	}

	if value, ok := params["include_stats"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			result.IncludeStats = parsed
		}
	}

	if value, ok := params["include_io"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			result.IncludeIO = parsed
		}
	}

	if value, ok := params["include_net"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			result.IncludeNet = parsed
		}
	}

	if value, ok := params["ascending"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			result.Ascending = parsed
		}
	}

	if value, ok := params["compact"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			result.Compact = parsed
		}
	}

	if value, ok := params["max_cmd_length"]; ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			result.MaxCmdLineLength = parsed
		}
	}

	if value, ok := params["limit"]; ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			result.Limit = parsed
		}
	}

	if value, ok := params["sort_by"]; ok {
		result.SortBy = value
	}

	if value, ok := params["scrub_args"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			result.ScrubArgs = parsed
		}
	}

	if result.Limit > defaultMaxProcessSnapshot {
		result.Limit = defaultMaxProcessSnapshot
	}

	return result
}

func parseInt32List(value string) []int32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var result []int32
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		trimmed := strings.TrimSpace(value[1 : len(value)-1])
		if trimmed == "" {
			return nil
		}
		if strings.Contains(trimmed, ",") {
			fields := strings.Split(trimmed, ",")
			for _, field := range fields {
				if parsed, err := strconv.Atoi(strings.TrimSpace(field)); err == nil {
					result = append(result, int32(parsed))
				}
			}
			return result
		}
		fields := strings.Fields(trimmed)
		for _, field := range fields {
			if parsed, err := strconv.Atoi(strings.TrimSpace(field)); err == nil {
				result = append(result, int32(parsed))
			}
		}
		return result
	}

	if err := json.Unmarshal([]byte(value), &result); err == nil {
		return result
	}

	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	for _, field := range fields {
		if parsed, err := strconv.Atoi(strings.TrimSpace(field)); err == nil {
			result = append(result, int32(parsed))
		}
	}
	return result
}

func parseStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var result []string
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		trimmed := strings.TrimSpace(value[1 : len(value)-1])
		if trimmed == "" {
			return nil
		}
		if strings.Contains(trimmed, ",") {
			fields := strings.Split(trimmed, ",")
			for _, field := range fields {
				field = strings.TrimSpace(field)
				if field != "" {
					result = append(result, strings.Trim(field, `"'`))
				}
			}
			return result
		}
		fields := strings.Fields(trimmed)
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field != "" {
				result = append(result, strings.Trim(field, `"'`))
			}
		}
		return result
	}

	if err := json.Unmarshal([]byte(value), &result); err == nil {
		return result
	}

	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\t' || r == '\n'
	})
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}

func applyProcessFilters(procs []*procutil.Process, params *ProcessParams) []*procutil.Process {
	if len(params.PIDs) == 0 && len(params.ProcessNames) == 0 && params.RegexFilter == "" {
		return procs
	}

	filtered := make([]*procutil.Process, 0, len(procs))

	pidSet := make(map[int32]bool, len(params.PIDs))
	for _, pid := range params.PIDs {
		pidSet[pid] = true
	}

	nameSet := make(map[string]bool, len(params.ProcessNames))
	for _, name := range params.ProcessNames {
		nameSet[strings.ToLower(name)] = true
	}

	var regexPattern *regexp.Regexp
	if params.RegexFilter != "" {
		if compiled, err := regexp.Compile(params.RegexFilter); err == nil {
			regexPattern = compiled
		}
	}

	for _, proc := range procs {
		if len(params.PIDs) > 0 && !pidSet[proc.Pid] {
			continue
		}

		if len(params.ProcessNames) > 0 && !nameSet[strings.ToLower(proc.Name)] {
			continue
		}

		if regexPattern != nil {
			matched := regexPattern.MatchString(proc.Name)
			if !matched {
				for _, arg := range proc.Cmdline {
					if regexPattern.MatchString(arg) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}

		filtered = append(filtered, proc)
	}

	return filtered
}

func scrubProcesses(scrubber *procutil.DataScrubber, procs []*procutil.Process) {
	for _, proc := range procs {
		if len(proc.Cmdline) > 0 {
			scrubbed := scrubber.ScrubProcessCommand(proc)
			if len(scrubbed) > 0 {
				proc.Cmdline = scrubbed
			}
		}
	}
}

func sortProcesses(procs []*procutil.Process, params *ProcessParams) {
	if params.SortBy == "" {
		return
	}

	sort.Slice(procs, func(i, j int) bool {
		var less bool
		switch params.SortBy {
		case "pid":
			less = procs[i].Pid < procs[j].Pid
		case "name":
			less = strings.ToLower(procs[i].Name) < strings.ToLower(procs[j].Name)
		case "cpu":
			cpuI := float64(0)
			cpuJ := float64(0)
			if procs[i].Stats != nil && procs[i].Stats.CPUPercent != nil {
				cpuI = procs[i].Stats.CPUPercent.UserPct + procs[i].Stats.CPUPercent.SystemPct
			}
			if procs[j].Stats != nil && procs[j].Stats.CPUPercent != nil {
				cpuJ = procs[j].Stats.CPUPercent.UserPct + procs[j].Stats.CPUPercent.SystemPct
			}
			less = cpuI < cpuJ
		case "memory":
			memI := uint64(0)
			memJ := uint64(0)
			if procs[i].Stats != nil && procs[i].Stats.MemInfo != nil {
				memI = procs[i].Stats.MemInfo.RSS
			}
			if procs[j].Stats != nil && procs[j].Stats.MemInfo != nil {
				memJ = procs[j].Stats.MemInfo.RSS
			}
			less = memI < memJ
		default:
			less = procs[i].Pid < procs[j].Pid
		}

		if params.Ascending {
			return less
		}
		return !less
	})
}

func buildProcessSnapshot(procs []*procutil.Process, now time.Time, totalCount, filteredCount int, params *ProcessParams) *ProcessSnapshot {
	snapshot := &ProcessSnapshot{
		Timestamp:     now.Unix(),
		TotalCount:    totalCount,
		FilteredCount: filteredCount,
	}

	if !params.Compact {
		snapshot.HostInfo = getHostMetadata()
	}

	if params.Compact {
		snapshot.CompactProcesses = make([]*CompactProcessInfo, 0, len(procs))
		for _, proc := range procs {
			snapshot.CompactProcesses = append(snapshot.CompactProcesses, convertProcessCompact(proc, params.MaxCmdLineLength))
		}
	} else {
		snapshot.Processes = make([]*ProcessInfo, 0, len(procs))
		for _, proc := range procs {
			snapshot.Processes = append(snapshot.Processes, convertProcess(proc, params.MaxCmdLineLength))
		}
	}

	return snapshot
}

func convertProcess(proc *procutil.Process, maxCmdLen int) *ProcessInfo {
	info := &ProcessInfo{
		PID:         proc.Pid,
		PPID:        proc.Ppid,
		Name:        proc.Name,
		Executable:  proc.Exe,
		CommandLine: truncateCmdLine(proc.Cmdline, maxCmdLen),
		Username:    proc.Username,
	}

	if len(proc.Uids) > 0 {
		info.UserID = proc.Uids[0]
	}
	if len(proc.Gids) > 0 {
		info.GroupID = proc.Gids[0]
	}

	if proc.PortsCollected {
		info.TCPPorts = proc.TCPPorts
		info.UDPPorts = proc.UDPPorts
		info.Connections = int32(len(proc.TCPPorts) + len(proc.UDPPorts))
	}

	if proc.InjectionState == procutil.InjectionInjected {
		info.APMInjected = true
	}

	if proc.Stats != nil {
		stats := proc.Stats

		info.CreateTime = stats.CreateTime
		info.Status = stats.Status
		info.OpenFiles = stats.OpenFdCount
		info.NumThreads = stats.NumThreads

		if stats.CPUPercent != nil {
			info.CPUPercent = stats.CPUPercent.UserPct + stats.CPUPercent.SystemPct
		}

		if stats.CPUTime != nil {
			info.CPUTime = &CPUTime{
				User:   stats.CPUTime.User,
				System: stats.CPUTime.System,
			}
		}

		if stats.MemInfo != nil {
			memInfo := &MemoryInfo{
				RSS: stats.MemInfo.RSS,
				VMS: stats.MemInfo.VMS,
			}
			if stats.MemInfoEx != nil {
				memInfo.Shared = stats.MemInfoEx.Shared
				memInfo.Text = stats.MemInfoEx.Text
				memInfo.Data = stats.MemInfoEx.Data
			}
			if memoryTotal, err := mem.VirtualMemory(); err == nil && memoryTotal.Total > 0 {
				memInfo.Percent = float32(float64(stats.MemInfo.RSS) / float64(memoryTotal.Total) * 100)
			}
			info.Memory = memInfo
		}

		if stats.IOStat != nil {
			info.IOCounters = &IOInfo{
				ReadCount:  uint64(stats.IOStat.ReadCount),
				WriteCount: uint64(stats.IOStat.WriteCount),
				ReadBytes:  uint64(stats.IOStat.ReadBytes),
				WriteBytes: uint64(stats.IOStat.WriteBytes),
			}

			if stats.IORateStat != nil {
				info.IOCounters.ReadRate = stats.IORateStat.ReadBytesRate
				info.IOCounters.WriteRate = stats.IORateStat.WriteBytesRate
			}
		}
	}

	if proc.Service != nil {
		info.Service = &ServiceInfo{
			Name:        proc.Service.GeneratedName,
			DisplayName: proc.Service.GeneratedName,
			DDService:   proc.Service.DDService,
		}

		if proc.Language != nil {
			info.Service.Language = string(proc.Language.Name)
		}
	}

	return info
}

func getHostMetadata() *HostMetadata {
	metadata := &HostMetadata{}

	if hn, err := hostname.Get(context.Background()); err == nil {
		metadata.Hostname = hn
	}

	if hostInfo, err := host.Info(); err == nil {
		metadata.OS = hostInfo.OS
		metadata.Platform = hostInfo.Platform
		metadata.KernelVersion = hostInfo.KernelVersion
	}

	if cpuCores, err := cpu.Counts(true); err == nil {
		metadata.CPUCores = cpuCores
	}

	if memInfo, err := mem.VirtualMemory(); err == nil {
		metadata.TotalMemory = memInfo.Total
	}

	return metadata
}

func convertProcessCompact(proc *procutil.Process, maxCmdLen int) *CompactProcessInfo {
	info := &CompactProcessInfo{
		PID:  proc.Pid,
		Name: proc.Name,
	}

	if len(proc.Cmdline) > 0 {
		cmd := strings.Join(proc.Cmdline, " ")
		if maxCmdLen > 0 && len(cmd) > maxCmdLen {
			cmd = cmd[:maxCmdLen] + "..."
		}
		info.Cmd = cmd
	}

	if proc.Stats != nil {
		if proc.Stats.CPUPercent != nil {
			info.CPUPercent = proc.Stats.CPUPercent.UserPct + proc.Stats.CPUPercent.SystemPct
		}
		if proc.Stats.MemInfo != nil {
			info.MemRSS = proc.Stats.MemInfo.RSS
			if memoryTotal, err := mem.VirtualMemory(); err == nil && memoryTotal.Total > 0 {
				info.MemPercent = float32(float64(proc.Stats.MemInfo.RSS) / float64(memoryTotal.Total) * 100)
			}
		}
	}

	return info
}

func truncateCmdLine(cmdline []string, maxLen int) []string {
	if maxLen <= 0 || len(cmdline) == 0 {
		return cmdline
	}

	totalLen := 0
	for _, arg := range cmdline {
		totalLen += len(arg)
	}

	if totalLen <= maxLen {
		return cmdline
	}

	result := make([]string, 0, len(cmdline))
	remaining := maxLen

	for i, arg := range cmdline {
		if remaining <= 0 {
			result = append(result, "...")
			break
		}
		if len(arg) <= remaining {
			result = append(result, arg)
			remaining -= len(arg)
		} else {
			if i == 0 && remaining > 10 {
				result = append(result, arg[:remaining]+"...")
			} else if remaining > 3 {
				result = append(result, arg[:remaining-3]+"...")
			}
			break
		}
	}

	return result
}
