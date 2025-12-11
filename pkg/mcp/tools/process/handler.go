// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/mcp/types"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/gopsutil/host"
	"github.com/DataDog/gopsutil/mem"
	"github.com/DataDog/gopsutil/cpu"
)

// ProcessHandler implements the process monitoring MCP tool
type ProcessHandler struct {
	probe    procutil.Probe
	scrubber *procutil.DataScrubber
	config   pkgconfigmodel.Reader
}

// NewProcessHandler creates a new process tool handler
func NewProcessHandler(cfg pkgconfigmodel.Reader) (*ProcessHandler, error) {
	probe := procutil.NewProcessProbe()
	scrubber := procutil.NewDefaultDataScrubber()

	return &ProcessHandler{
		probe:    probe,
		scrubber: scrubber,
		config:   cfg,
	}, nil
}

// Handle processes GetProcessSnapshot requests
func (h *ProcessHandler) Handle(ctx context.Context, req *types.ToolRequest) (*types.ToolResponse, error) {
	// Parse request parameters
	params, err := h.parseProcessParams(req.Parameters)
	if err != nil {
		return &types.ToolResponse{
			ToolName:  req.ToolName,
			Error:     fmt.Sprintf("invalid parameters: %v", err),
			RequestID: req.RequestID,
		}, fmt.Errorf("invalid parameters: %w", err)
	}

	// Get process snapshot
	snapshot, err := h.getSnapshot(ctx, params)
	if err != nil {
		return &types.ToolResponse{
			ToolName:  req.ToolName,
			Error:     fmt.Sprintf("failed to get process snapshot: %v", err),
			RequestID: req.RequestID,
		}, fmt.Errorf("failed to get process snapshot: %w", err)
	}

	return &types.ToolResponse{
		ToolName:  req.ToolName,
		Result:    snapshot,
		RequestID: req.RequestID,
	}, nil
}

func (h *ProcessHandler) parseProcessParams(params map[string]interface{}) (*ProcessParams, error) {
	result := &ProcessParams{
		IncludeStats:     true, // Default to include stats
		Limit:            25,   // Default limit (reduced for smaller responses)
		Compact:          true, // Default to compact mode for smaller responses
		MaxCmdLineLength: 200,  // Truncate long command lines
	}

	// Parse PIDs
	if pids, ok := params["pids"]; ok {
		if pidList, ok := pids.([]interface{}); ok {
			result.PIDs = make([]int32, 0, len(pidList))
			for _, p := range pidList {
				switch v := p.(type) {
				case float64:
					result.PIDs = append(result.PIDs, int32(v))
				case int:
					result.PIDs = append(result.PIDs, int32(v))
				case int32:
					result.PIDs = append(result.PIDs, v)
				}
			}
		}
	}

	// Parse process names
	if names, ok := params["process_names"]; ok {
		if nameList, ok := names.([]interface{}); ok {
			result.ProcessNames = make([]string, 0, len(nameList))
			for _, n := range nameList {
				if str, ok := n.(string); ok {
					result.ProcessNames = append(result.ProcessNames, str)
				}
			}
		}
	}

	// Parse regex filter
	if regex, ok := params["regex_filter"]; ok {
		if str, ok := regex.(string); ok {
			result.RegexFilter = str
		}
	}

	// Parse boolean flags
	if includeStats, ok := params["include_stats"]; ok {
		if b, ok := includeStats.(bool); ok {
			result.IncludeStats = b
		}
	}

	if includeIO, ok := params["include_io"]; ok {
		if b, ok := includeIO.(bool); ok {
			result.IncludeIO = b
		}
	}

	if includeNet, ok := params["include_net"]; ok {
		if b, ok := includeNet.(bool); ok {
			result.IncludeNet = b
		}
	}

	if ascending, ok := params["ascending"]; ok {
		if b, ok := ascending.(bool); ok {
			result.Ascending = b
		}
	}

	// Parse compact mode (default true for smaller responses)
	if compact, ok := params["compact"]; ok {
		if b, ok := compact.(bool); ok {
			result.Compact = b
		}
	}

	// Parse max command line length
	if maxCmdLen, ok := params["max_cmd_length"]; ok {
		switch v := maxCmdLen.(type) {
		case float64:
			result.MaxCmdLineLength = int(v)
		case int:
			result.MaxCmdLineLength = v
		}
	}

	// Parse limit
	if limit, ok := params["limit"]; ok {
		switch v := limit.(type) {
		case float64:
			result.Limit = int(v)
		case int:
			result.Limit = v
		}
	}

	// Parse sort_by
	if sortBy, ok := params["sort_by"]; ok {
		if str, ok := sortBy.(string); ok {
			result.SortBy = str
		}
	}

	// Validate limit
	maxLimit := h.config.GetInt("mcp.tools.process.max_processes_per_request")
	if maxLimit == 0 {
		maxLimit = 1000 // Default max
	}
	if result.Limit > maxLimit {
		result.Limit = maxLimit
	}

	return result, nil
}

func (h *ProcessHandler) getSnapshot(ctx context.Context, params *ProcessParams) (*ProcessSnapshot, error) {
	now := time.Now()

	// Get all processes with stats
	procs, err := h.probe.ProcessesByPID(now, params.IncludeStats)
	if err != nil {
		return nil, err
	}

	totalCount := len(procs)

	// Convert to slice for filtering
	processList := make([]*procutil.Process, 0, len(procs))
	for _, p := range procs {
		processList = append(processList, p)
	}

	// Apply filters
	filtered := h.applyFilters(processList, params)

	// Scrub sensitive data if configured
	if h.config.GetBool("mcp.tools.process.scrub_args") {
		h.scrubProcesses(filtered)
	}

	// Sort processes
	h.sortProcesses(filtered, params)

	// Apply limit
	if params.Limit > 0 && len(filtered) > params.Limit {
		filtered = filtered[:params.Limit]
	}

	// Build response
	snapshot := h.buildSnapshot(filtered, now, totalCount, len(filtered), params)

	return snapshot, nil
}

func (h *ProcessHandler) applyFilters(procs []*procutil.Process, params *ProcessParams) []*procutil.Process {
	if len(params.PIDs) == 0 && len(params.ProcessNames) == 0 && params.RegexFilter == "" {
		return procs
	}

	filtered := make([]*procutil.Process, 0, len(procs))

	// Build PID lookup
	pidSet := make(map[int32]bool)
	for _, pid := range params.PIDs {
		pidSet[pid] = true
	}

	// Build name lookup
	nameSet := make(map[string]bool)
	for _, name := range params.ProcessNames {
		nameSet[strings.ToLower(name)] = true
	}

	// Compile regex if provided
	var regexPattern *regexp.Regexp
	if params.RegexFilter != "" {
		var err error
		regexPattern, err = regexp.Compile(params.RegexFilter)
		if err != nil {
			// If regex is invalid, ignore it
			regexPattern = nil
		}
	}

	for _, proc := range procs {
		// Filter by PID
		if len(params.PIDs) > 0 {
			if !pidSet[proc.Pid] {
				continue
			}
		}

		// Filter by process name
		if len(params.ProcessNames) > 0 {
			if !nameSet[strings.ToLower(proc.Name)] {
				continue
			}
		}

		// Filter by regex
		if regexPattern != nil {
			matched := false
			// Check name
			if regexPattern.MatchString(proc.Name) {
				matched = true
			}
			// Check command line
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

func (h *ProcessHandler) scrubProcesses(procs []*procutil.Process) {
	for _, proc := range procs {
		if len(proc.Cmdline) > 0 {
			scrubbed := h.scrubber.ScrubProcessCommand(proc)
			if scrubbed != nil && len(scrubbed) > 0 {
				proc.Cmdline = scrubbed
			}
		}
	}
}

func (h *ProcessHandler) sortProcesses(procs []*procutil.Process, params *ProcessParams) {
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

func (h *ProcessHandler) buildSnapshot(procs []*procutil.Process, now time.Time, totalCount, filteredCount int, params *ProcessParams) *ProcessSnapshot {
	snapshot := &ProcessSnapshot{
		Timestamp:     now.Unix(),
		TotalCount:    totalCount,
		FilteredCount: filteredCount,
	}

	// Only include host info in non-compact mode
	if !params.Compact {
		snapshot.HostInfo = h.getHostMetadata()
	}

	if params.Compact {
		// Compact mode: minimal fields
		snapshot.CompactProcesses = make([]*CompactProcessInfo, 0, len(procs))
		for _, proc := range procs {
			compact := h.convertProcessCompact(proc, params.MaxCmdLineLength)
			snapshot.CompactProcesses = append(snapshot.CompactProcesses, compact)
		}
	} else {
		// Full mode: all fields
		snapshot.Processes = make([]*ProcessInfo, 0, len(procs))
		for _, proc := range procs {
			processInfo := h.convertProcess(proc, params.MaxCmdLineLength)
			snapshot.Processes = append(snapshot.Processes, processInfo)
		}
	}

	return snapshot
}

func (h *ProcessHandler) convertProcess(proc *procutil.Process, maxCmdLen int) *ProcessInfo {
	info := &ProcessInfo{
		PID:         proc.Pid,
		PPID:        proc.Ppid,
		Name:        proc.Name,
		Executable:  proc.Exe,
		CommandLine: truncateCmdLine(proc.Cmdline, maxCmdLen),
		Username:    proc.Username,
	}

	// Set UIDs/GIDs
	if len(proc.Uids) > 0 {
		info.UserID = proc.Uids[0]
	}
	if len(proc.Gids) > 0 {
		info.GroupID = proc.Gids[0]
	}

	// Add ports if available
	if proc.PortsCollected {
		info.TCPPorts = proc.TCPPorts
		info.UDPPorts = proc.UDPPorts
		info.Connections = int32(len(proc.TCPPorts) + len(proc.UDPPorts))
	}

	// Add APM injection status
	if proc.InjectionState == procutil.InjectionInjected {
		info.APMInjected = true
	}

	// Add stats if available
	if proc.Stats != nil {
		stats := proc.Stats

		info.CreateTime = stats.CreateTime
		info.Status = stats.Status
		info.OpenFiles = stats.OpenFdCount
		info.NumThreads = stats.NumThreads

		// CPU stats
		if stats.CPUPercent != nil {
			info.CPUPercent = stats.CPUPercent.UserPct + stats.CPUPercent.SystemPct
		}

		if stats.CPUTime != nil {
			info.CPUTime = &CPUTime{
				User:   stats.CPUTime.User,
				System: stats.CPUTime.System,
			}
		}

		// Memory stats
		if stats.MemInfo != nil {
			memInfo := &MemoryInfo{
				RSS: stats.MemInfo.RSS,
				VMS: stats.MemInfo.VMS,
			}
			// Get extended memory info if available
			if stats.MemInfoEx != nil {
				memInfo.Shared = stats.MemInfoEx.Shared
				memInfo.Text = stats.MemInfoEx.Text
				memInfo.Data = stats.MemInfoEx.Data
			}
			// Calculate memory percentage (RSS / total memory)
			if memoryTotal, err := mem.VirtualMemory(); err == nil && memoryTotal.Total > 0 {
				memInfo.Percent = float32(float64(stats.MemInfo.RSS) / float64(memoryTotal.Total) * 100)
			}
			info.Memory = memInfo
		}

		// IO stats
		if stats.IOStat != nil {
			info.IOCounters = &IOInfo{
				ReadCount:  uint64(stats.IOStat.ReadCount),
				WriteCount: uint64(stats.IOStat.WriteCount),
				ReadBytes:  uint64(stats.IOStat.ReadBytes),
				WriteBytes: uint64(stats.IOStat.WriteBytes),
			}

			// Add IO rate if available
			if stats.IORateStat != nil {
				info.IOCounters.ReadRate = stats.IORateStat.ReadBytesRate
				info.IOCounters.WriteRate = stats.IORateStat.WriteBytesRate
			}
		}
	}

	// Add service info if available
	if proc.Service != nil {
		info.Service = &ServiceInfo{
			Name:        proc.Service.GeneratedName,
			DisplayName: proc.Service.GeneratedName,
			DDService:   proc.Service.DDService,
		}

		// Add language if detected
		if proc.Language != nil {
			info.Service.Language = string(string(proc.Language.Name))
		}
	}

	return info
}

func (h *ProcessHandler) getHostMetadata() *HostMetadata {
	metadata := &HostMetadata{}

	// Get hostname
	if hn, err := hostname.Get(context.Background()); err == nil {
		metadata.Hostname = hn
	}

	// Get host info
	if hostInfo, err := host.Info(); err == nil {
		metadata.OS = hostInfo.OS
		metadata.Platform = hostInfo.Platform
		metadata.KernelVersion = hostInfo.KernelVersion
	}

	// Get CPU cores

	// Get CPU cores
	if cpuCores, err := cpu.Counts(true); err == nil {
		metadata.CPUCores = cpuCores
	}

	// Get total memory
	if memInfo, err := mem.VirtualMemory(); err == nil {
		metadata.TotalMemory = memInfo.Total
	}

	return metadata
}

// convertProcessCompact creates a minimal process representation
func (h *ProcessHandler) convertProcessCompact(proc *procutil.Process, maxCmdLen int) *CompactProcessInfo {
	info := &CompactProcessInfo{
		PID:  proc.Pid,
		Name: proc.Name,
	}

	// Truncated command line as single string
	if len(proc.Cmdline) > 0 {
		cmd := strings.Join(proc.Cmdline, " ")
		if maxCmdLen > 0 && len(cmd) > maxCmdLen {
			cmd = cmd[:maxCmdLen] + "..."
		}
		info.Cmd = cmd
	}

	// Add stats if available
	if proc.Stats != nil {
		if proc.Stats.CPUPercent != nil {
			info.CPUPercent = proc.Stats.CPUPercent.UserPct + proc.Stats.CPUPercent.SystemPct
		}
		if proc.Stats.MemInfo != nil {
			info.MemRSS = proc.Stats.MemInfo.RSS
			// Calculate memory percentage
			if memoryTotal, err := mem.VirtualMemory(); err == nil && memoryTotal.Total > 0 {
				info.MemPercent = float32(float64(proc.Stats.MemInfo.RSS) / float64(memoryTotal.Total) * 100)
			}
		}
	}

	return info
}

// truncateCmdLine truncates command line arguments to a maximum total length
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

	// Truncate: keep first arg (executable), truncate rest
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
			// Truncate this argument
			if i == 0 && remaining > 10 {
				// Keep more of the executable name
				result = append(result, arg[:remaining]+"...")
			} else if remaining > 3 {
				result = append(result, arg[:remaining-3]+"...")
			}
			break
		}
	}

	return result
}

// Close closes the process handler and releases resources
func (h *ProcessHandler) Close() {
	if h.probe != nil {
		h.probe.Close()
	}
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
