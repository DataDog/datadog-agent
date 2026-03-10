// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nccl

import (
	"bytes"
	"encoding/json"
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
	ChannelID int   `json:"channel_id"`               // Channel ID
	Peer      int   `json:"peer"`                     // Remote rank
	NSteps    int   `json:"n_steps"`                  // Number of steps
	ChunkSize int   `json:"chunk_size"`               // Chunk size in bytes
	IsSend    int   `json:"is_send"`                  // 1=send, 0=recv
	StartUS   int64 `json:"start_us"`                 // Start timestamp (microseconds)
	StopUS    int64 `json:"stop_us"`                  // Stop timestamp (microseconds)
	NetTimeUS int64 `json:"proxy_op_network_time_us"` // Proxy operation network time (stop - start)
}

// nvidiaInspectorEvent is the output format of NVIDIA's official NCCL Inspector
// plugin (format version v4.0+). It uses a nested header/metadata structure.
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
// Supports NVIDIA's nested format (detected by "header" key) and flat format.
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

// ParsedEvent combines NCCL Inspector event with metadata
type ParsedEvent struct {
	Event     NCCLInspectorEvent
	Filename  string
	ParseTime time.Time
	HostPID   int // kernel-provided host-namespace PID from SO_PEERCRED; 0 for file-based events
}
