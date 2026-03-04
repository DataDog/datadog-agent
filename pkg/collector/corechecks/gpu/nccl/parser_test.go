// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nccl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNewEvents(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Write test JSON file
	testJSON := `{"id":"abc123","rank":0,"n_ranks":8,"nnodes":1,"pid":12345,"hostname":"worker-0","coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":1048576,"coll_exec_time_us":61974.5,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":277.21,"coll_busbw_gbs":485.12}}
{"id":"abc123","rank":1,"n_ranks":8,"nnodes":1,"pid":12346,"hostname":"worker-0","coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":1048576,"coll_exec_time_us":62100.2,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":275.50,"coll_busbw_gbs":482.00}}
`
	testFile := filepath.Join(tmpDir, "nccl_output.jsonl")
	err = os.WriteFile(testFile, []byte(testJSON), 0644)
	require.NoError(t, err)

	// Parse events
	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	assert.Len(t, events, 2)

	// Verify first event
	event0 := events[0].Event
	assert.Equal(t, "abc123", event0.ID)
	assert.Equal(t, 0, event0.Rank)
	assert.Equal(t, 8, event0.NRanks)
	assert.Equal(t, 12345, event0.PID)
	assert.Equal(t, "worker-0", event0.Hostname)
	require.NotNil(t, event0.CollPerf)
	assert.Equal(t, "AllReduce", event0.CollPerf.Collective)
	assert.InDelta(t, 61974.5, event0.CollPerf.ExecTimeUS, 0.1)
	assert.InDelta(t, 277.21, event0.CollPerf.AlgoBandwidthGB, 0.01)

	// Verify second event
	event1 := events[1].Event
	assert.Equal(t, 1, event1.Rank)
	assert.Equal(t, 12346, event1.PID)

	// Parse again - should return nothing (already processed)
	events2, err := parser.ParseNewEvents()
	require.NoError(t, err)
	assert.Len(t, events2, 0)
}

func TestParseIncrementalReads(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "nccl_output.jsonl")

	// Write first batch
	firstBatch := `{"id":"abc123","rank":0,"n_ranks":8,"nnodes":1,"pid":12345,"hostname":"worker-0","coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":1048576,"coll_exec_time_us":61974.5,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":277.21,"coll_busbw_gbs":485.12}}
`
	err = os.WriteFile(testFile, []byte(firstBatch), 0644)
	require.NoError(t, err)

	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	assert.Len(t, events, 1)

	// Append second batch
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString(`{"id":"abc123","rank":1,"n_ranks":8,"nnodes":1,"pid":12346,"hostname":"worker-0","coll_perf":{"coll":"AllGather","coll_sn":2,"coll_msg_size_bytes":2097152,"coll_exec_time_us":45000.0,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":300.00,"coll_busbw_gbs":500.00}}
`)
	require.NoError(t, err)
	f.Close()

	// Should only get the new event
	events2, err := parser.ParseNewEvents()
	require.NoError(t, err)
	assert.Len(t, events2, 1)
	assert.Equal(t, 1, events2[0].Event.Rank)
	assert.Equal(t, "AllGather", events2[0].Event.CollPerf.Collective)
}

func TestParseMalformedJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Write file with some malformed lines
	testJSON := `{"id":"abc123","rank":0,"n_ranks":8,"nnodes":1,"pid":12345,"hostname":"worker-0","coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":1048576,"coll_exec_time_us":61974.5,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":277.21,"coll_busbw_gbs":485.12}}
this is not json
{"id":"abc123","rank":1,"n_ranks":8,"nnodes":1,"pid":12346,"hostname":"worker-0","coll_perf":{"coll":"AllReduce","coll_sn":2,"coll_msg_size_bytes":1048576,"coll_exec_time_us":62000.0,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":276.00,"coll_busbw_gbs":483.00}}
`
	testFile := filepath.Join(tmpDir, "nccl_output.jsonl")
	err = os.WriteFile(testFile, []byte(testJSON), 0644)
	require.NoError(t, err)

	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	// Should skip the malformed line and parse the valid ones
	assert.Len(t, events, 2)
}

func TestParseEmptyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

func TestParseNvidiaInspectorFormat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// NVIDIA's official NCCL Inspector output format (v4.0+): nested header/metadata
	testJSON := `{"header":{"id":"abc123","rank":0,"n_ranks":3,"nnodes":3},"metadata":{"hostname":"worker-0","pid":12345,"dump_timestamp_us":1748030377748202,"gpu_uuid":"GPU-abc-123"},"coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":4194304,"coll_exec_time_us":4492.0,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":0.933,"coll_busbw_gbs":1.244}}
{"header":{"id":"abc123","rank":1,"n_ranks":3,"nnodes":3},"metadata":{"hostname":"worker-1","pid":12346,"dump_timestamp_us":1748030377748300,"gpu_uuid":"GPU-def-456"},"coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":4194304,"coll_exec_time_us":4510.0,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":0.930,"coll_busbw_gbs":1.240}}
`
	testFile := filepath.Join(tmpDir, "nccl_output.jsonl")
	err = os.WriteFile(testFile, []byte(testJSON), 0644)
	require.NoError(t, err)

	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Verify fields are correctly mapped from nested → flat
	event0 := events[0].Event
	assert.Equal(t, "abc123", event0.ID)
	assert.Equal(t, 0, event0.Rank)
	assert.Equal(t, 3, event0.NRanks)
	assert.Equal(t, 3, event0.NNodes)
	assert.Equal(t, 12345, event0.PID)
	assert.Equal(t, "worker-0", event0.Hostname)
	assert.Equal(t, "GPU-abc-123", event0.GPUUUID)
	require.NotNil(t, event0.CollPerf)
	assert.Equal(t, "AllReduce", event0.CollPerf.Collective)
	assert.InDelta(t, 4492.0, event0.CollPerf.ExecTimeUS, 0.1)
	assert.InDelta(t, 0.933, event0.CollPerf.AlgoBandwidthGB, 0.001)
	assert.InDelta(t, 1.244, event0.CollPerf.BusBandwidthGB, 0.001)
	assert.Equal(t, "kernel_gpu", event0.CollPerf.TimingSource)

	event1 := events[1].Event
	assert.Equal(t, 1, event1.Rank)
	assert.Equal(t, "worker-1", event1.Hostname)
}

func TestParseMixedFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Mix of NVIDIA nested format and our flat format in the same file
	testJSON := `{"header":{"id":"comm1","rank":0,"n_ranks":2,"nnodes":2},"metadata":{"hostname":"node-0","pid":100,"dump_timestamp_us":1000},"coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":1048576,"coll_exec_time_us":1000.0,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":1.0,"coll_busbw_gbs":1.0}}
{"id":"comm1","rank":1,"n_ranks":2,"nnodes":2,"pid":200,"hostname":"node-1","coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":1048576,"coll_exec_time_us":1050.0,"coll_timing_source":"kernel_cpu","coll_algobw_gbs":0.95,"coll_busbw_gbs":0.95}}
`
	testFile := filepath.Join(tmpDir, "nccl_output.jsonl")
	err = os.WriteFile(testFile, []byte(testJSON), 0644)
	require.NoError(t, err)

	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, 0, events[0].Event.Rank)
	assert.Equal(t, "kernel_gpu", events[0].Event.CollPerf.TimingSource)
	assert.Equal(t, 1, events[1].Event.Rank)
	assert.Equal(t, "kernel_cpu", events[1].Event.CollPerf.TimingSource)
}

func TestParseProxyOpEvent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// ProxyOp event in NVIDIA nested format (sent by our inspector_dd_socket.cc)
	testJSON := `{"header":{"id":"abc123","rank":0,"n_ranks":3,"nnodes":3},"metadata":{"dump_timestamp_us":1748030377748202,"hostname":"worker-0","pid":12345},"proxy_op":{"channel_id":0,"peer":1,"n_steps":8,"chunk_size":131072,"is_send":1,"start_us":1000000,"stop_us":1005000,"net_time_us":5000}}
{"header":{"id":"abc123","rank":0,"n_ranks":3,"nnodes":3},"metadata":{"dump_timestamp_us":1748030377748300,"hostname":"worker-0","pid":12345},"proxy_op":{"channel_id":0,"peer":2,"n_steps":8,"chunk_size":131072,"is_send":0,"start_us":1000100,"stop_us":1008000,"net_time_us":7900}}
`
	testFile := filepath.Join(tmpDir, "nccl_output.jsonl")
	err = os.WriteFile(testFile, []byte(testJSON), 0644)
	require.NoError(t, err)

	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	require.Len(t, events, 2)

	// First event: send to peer 1
	ev0 := events[0].Event
	assert.Equal(t, "abc123", ev0.ID)
	assert.Equal(t, 0, ev0.Rank)
	assert.Nil(t, ev0.CollPerf)
	require.NotNil(t, ev0.ProxyOp)
	assert.Equal(t, 0, ev0.ProxyOp.ChannelID)
	assert.Equal(t, 1, ev0.ProxyOp.Peer)
	assert.Equal(t, 1, ev0.ProxyOp.IsSend)
	assert.Equal(t, int64(5000), ev0.ProxyOp.NetTimeUS)

	// Second event: recv from peer 2
	ev1 := events[1].Event
	require.NotNil(t, ev1.ProxyOp)
	assert.Equal(t, 2, ev1.ProxyOp.Peer)
	assert.Equal(t, 0, ev1.ProxyOp.IsSend)
	assert.Equal(t, int64(7900), ev1.ProxyOp.NetTimeUS)
}

func TestParseEventWithoutCollPerf(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nccl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Event without coll_perf should be skipped
	testJSON := `{"id":"abc123","rank":0,"n_ranks":8,"nnodes":1,"pid":12345,"hostname":"worker-0"}
{"id":"abc123","rank":1,"n_ranks":8,"nnodes":1,"pid":12346,"hostname":"worker-0","coll_perf":{"coll":"AllReduce","coll_sn":1,"coll_msg_size_bytes":1048576,"coll_exec_time_us":61974.5,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":277.21,"coll_busbw_gbs":485.12}}
`
	testFile := filepath.Join(tmpDir, "nccl_output.jsonl")
	err = os.WriteFile(testFile, []byte(testJSON), 0644)
	require.NoError(t, err)

	parser := NewParser(tmpDir)
	events, err := parser.ParseNewEvents()
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, 1, events[0].Event.Rank)
}
