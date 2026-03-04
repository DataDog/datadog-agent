// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nccl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// NCCLInspectorEvent represents a single event from NCCL Inspector JSON output
// Based on NCCL Inspector v4.0 format
type NCCLInspectorEvent struct {
	// Header fields
	ID         string `json:"id"`          // Communicator hash
	Rank       int    `json:"rank"`        // Process rank in communicator
	NRanks     int    `json:"n_ranks"`     // Total number of ranks
	NNodes     int    `json:"nnodes"`      // Number of nodes
	PID        int    `json:"pid"`         // Process ID
	GlobalRank int    `json:"global_rank"` // Global rank across all nodes; -1 if not set
	GPUUUID    string `json:"gpu_uuid"`    // GPU UUID (if available)

	// Metadata
	Hostname        string `json:"hostname"`
	DumpTimestampUS int64  `json:"dump_timestamp_us"`

	// Collective performance data
	CollPerf *CollectivePerf `json:"coll_perf,omitempty"`

	// Proxy operation performance data (network transfer timing)
	ProxyOp *ProxyOpPerf `json:"proxy_op,omitempty"`
}

// CollectivePerf contains performance metrics for a collective operation
type CollectivePerf struct {
	Collective      string  `json:"coll"`                // Operation type: AllReduce, AllGather, etc.
	CollSN          int64   `json:"coll_sn"`             // Collective sequence number
	MsgSizeBytes    int64   `json:"coll_msg_size_bytes"` // Message size in bytes
	ExecTimeUS      float64 `json:"coll_exec_time_us"`   // Execution time in microseconds
	TimingSource    string  `json:"coll_timing_source"`  // kernel_gpu, kernel_cpu, collective_cpu
	AlgoBandwidthGB float64 `json:"coll_algobw_gbs"`     // Algorithmic bandwidth in GB/s
	BusBandwidthGB  float64 `json:"coll_busbw_gbs"`      // Bus bandwidth in GB/s
}

// ProxyOpPerf contains performance metrics for a proxy operation (network transfer)
type ProxyOpPerf struct {
	ChannelID int   `json:"channel_id"`  // Channel ID
	Peer      int   `json:"peer"`        // Remote rank
	NSteps    int   `json:"n_steps"`     // Number of steps
	ChunkSize int   `json:"chunk_size"`  // Chunk size in bytes
	IsSend    int   `json:"is_send"`     // 1=send, 0=recv
	StartUS   int64 `json:"start_us"`    // Start timestamp (microseconds)
	StopUS    int64 `json:"stop_us"`     // Stop timestamp (microseconds)
	NetTimeUS int64 `json:"net_time_us"` // Network transfer time (stop - start)
}

// nvidiaInspectorEvent is the output format of NVIDIA's official NCCL Inspector
// plugin (format version v4.0+). It uses a nested header/metadata structure.
// Reference: https://developer.nvidia.com/blog/enhancing-communication-observability-of-ai-workloads-with-nccl-inspector/
type nvidiaInspectorEvent struct {
	Header   nvidiaHeader    `json:"header"`
	Metadata nvidiaMetadata  `json:"metadata"`
	CollPerf *CollectivePerf `json:"coll_perf,omitempty"`
	ProxyOp  *ProxyOpPerf    `json:"proxy_op,omitempty"`
}

type nvidiaHeader struct {
	ID     string `json:"id"`
	Rank   int    `json:"rank"`
	NRanks int    `json:"n_ranks"`
	NNodes int    `json:"nnodes"`
}

type nvidiaMetadata struct {
	Hostname        string `json:"hostname"`
	PID             int    `json:"pid"`
	DumpTimestampUS int64  `json:"dump_timestamp_us"`
	GPUUUID         string `json:"gpu_uuid"`
}

// toNCCLInspectorEvent converts NVIDIA's nested format to our internal representation.
// GlobalRank is set to -1 because NVIDIA's format does not include this field.
func (e *nvidiaInspectorEvent) toNCCLInspectorEvent() NCCLInspectorEvent {
	return NCCLInspectorEvent{
		ID:              e.Header.ID,
		Rank:            e.Header.Rank,
		NRanks:          e.Header.NRanks,
		NNodes:          e.Header.NNodes,
		PID:             e.Metadata.PID,
		GlobalRank:      -1,
		GPUUUID:         e.Metadata.GPUUUID,
		Hostname:        e.Metadata.Hostname,
		DumpTimestampUS: e.Metadata.DumpTimestampUS,
		CollPerf:        e.CollPerf,
		ProxyOp:         e.ProxyOp,
	}
}

// parseEvent parses a single JSONL line into an NCCLInspectorEvent.
// It supports two formats:
//   - NVIDIA's official nested format (detected by the presence of a "header" key)
//   - Our flat format (written by nccl_inspector.cpp)
//
// NVIDIA's format is tried first; if it is detected but fails to parse, the line
// is not retried as flat (it is likely a malformed NVIDIA line).
func parseEvent(line []byte) (NCCLInspectorEvent, error) {
	if bytes.Contains(line, []byte(`"header"`)) {
		var nvidia nvidiaInspectorEvent
		if err := json.Unmarshal(line, &nvidia); err != nil {
			return NCCLInspectorEvent{}, err
		}
		return nvidia.toNCCLInspectorEvent(), nil
	}
	var event NCCLInspectorEvent
	err := json.Unmarshal(line, &event)
	return event, err
}

// ParsedEvent combines NCCL Inspector event with file metadata
type ParsedEvent struct {
	Event     NCCLInspectorEvent
	Filename  string
	ParseTime time.Time
	HostPID   int // kernel-provided host-namespace PID from SO_PEERCRED; 0 for file-based events
}

// Parser handles reading and parsing NCCL Inspector JSON files
type Parser struct {
	jsonDir        string
	processedFiles map[string]int64 // filename -> last processed position
}

// NewParser creates a new NCCL Inspector JSON parser
func NewParser(jsonDir string) *Parser {
	return &Parser{
		jsonDir:        jsonDir,
		processedFiles: make(map[string]int64),
	}
}

// ParseNewEvents reads new events from JSON files in the configured directory
// Returns events that haven't been processed yet
func (p *Parser) ParseNewEvents() ([]ParsedEvent, error) {
	var events []ParsedEvent

	// Find all JSON files
	pattern := filepath.Join(p.jsonDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob JSON files: %w", err)
	}

	// Also check for JSONL files (JSON Lines format)
	patternJSONL := filepath.Join(p.jsonDir, "*.jsonl")
	jsonlFiles, err := filepath.Glob(patternJSONL)
	if err != nil {
		return nil, fmt.Errorf("failed to glob JSONL files: %w", err)
	}
	files = append(files, jsonlFiles...)

	for _, file := range files {
		fileEvents, err := p.parseFile(file)
		if err != nil {
			// Log but continue processing other files
			continue
		}
		events = append(events, fileEvents...)
	}

	return events, nil
}

// parseFile parses a single JSON/JSONL file, tracking position for incremental reads
func (p *Parser) parseFile(filename string) ([]ParsedEvent, error) {
	var events []ParsedEvent

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	// Get file info for size check
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", filename, err)
	}

	// Check if we've processed this file before
	lastPos, exists := p.processedFiles[filename]
	if exists {
		// If file is smaller than last position, it was truncated - start from beginning
		if info.Size() < lastPos {
			lastPos = 0
		}
		// Seek to last processed position
		if _, err := file.Seek(lastPos, 0); err != nil {
			return nil, fmt.Errorf("failed to seek in file %s: %w", filename, err)
		}
	}

	// Parse line by line (JSONL format - one JSON object per line)
	scanner := bufio.NewScanner(file)
	// Increase buffer size for potentially large JSON lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	parseTime := time.Now()
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := parseEvent(line)
		if err != nil {
			// Skip malformed lines
			continue
		}

		// Only include events with collective or proxy_op performance data
		if event.CollPerf != nil || event.ProxyOp != nil {
			events = append(events, ParsedEvent{
				Event:     event,
				Filename:  filename,
				ParseTime: parseTime,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("error scanning file %s: %w", filename, err)
	}

	// Update processed position
	newPos, _ := file.Seek(0, 1) // Get current position
	p.processedFiles[filename] = newPos

	return events, nil
}

// CleanupOldFiles removes files older than the retention period
func (p *Parser) CleanupOldFiles(retention time.Duration) error {
	pattern := filepath.Join(p.jsonDir, "*.json*")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob files for cleanup: %w", err)
	}

	cutoff := time.Now().Add(-retention)
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(file); err != nil {
				// Log but continue
				continue
			}
			// Remove from processed files map
			delete(p.processedFiles, file)
		}
	}

	return nil
}

// Reset clears the processed files tracking
func (p *Parser) Reset() {
	p.processedFiles = make(map[string]int64)
}
