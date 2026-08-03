// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nccl

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ncclSpanType    = "gpu"
	ncclSpanService = "nccl"
)

type spanEmitter struct {
	client *http.Client
	url    string
}

func newSpanEmitter(port int) *spanEmitter {
	return &spanEmitter{
		client: &http.Client{Timeout: 2 * time.Second},
		url:    fmt.Sprintf("http://127.0.0.1:%d/v0.4/traces", port),
	}
}

// emitTraces sends a batch of traces to the local trace agent in one POST.
func (se *spanEmitter) emitTraces(traces pb.Traces) error {
	payload, err := traces.MarshalMsg(nil)
	if err != nil {
		return fmt.Errorf("marshal traces: %w", err)
	}
	log.Debugf("NCCL span emitter: posting %d traces (%d bytes) to %s", len(traces), len(payload), se.url)
	req, err := http.NewRequest(http.MethodPost, se.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/msgpack")
	req.Header.Set("X-Datadog-Trace-Count", strconv.Itoa(len(traces)))
	resp, err := se.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", se.url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Debugf("NCCL span emitter: response HTTP %d: %s", resp.StatusCode, body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s: HTTP %d: %s", se.url, resp.StatusCode, body)
	}
	return nil
}

// buildSpan constructs an APM span from a collective event and its tags.
// tags is the same slice built by processEvent (UST + NCCL-specific tags).
func buildSpan(event NCCLInspectorEvent, tags []string) *pb.Span {
	perf := event.CollPerf

	durationNS := int64(perf.ExecTimeUS * 1000)
	startNS := event.DumpTimestampUS*1000 - durationNS

	service := extractTag(tags, "service")
	if service == "" {
		service = ncclSpanService
	}

	// All tags become span metadata for filtering in Trace Explorer.
	meta := make(map[string]string, len(tags)+4)
	for _, t := range tags {
		if k, v, ok := strings.Cut(t, ":"); ok {
			meta[k] = v
		}
	}
	meta["nccl.comm_id"] = event.ID
	if perf.TimingSource != "" {
		meta["nccl.timing_source"] = perf.TimingSource
	}
	if event.Hostname != "" {
		meta["nccl.hostname"] = event.Hostname
	}
	if event.GPUUUID != "" {
		meta["nccl.gpu_uuid"] = event.GPUUUID
	}

	return &pb.Span{
		Service:  service,
		Name:     perf.Collective,
		Resource: fmt.Sprintf("rank:%d", event.Rank),
		TraceID:  collTraceID(event.ID, perf.CollSN),
		SpanID:   collSpanID(event.ID, event.Rank, perf.CollSN),
		ParentID: 0,
		Start:    startNS,
		Duration: durationNS,
		Meta:     meta,
		Metrics: map[string]float64{
			"_sampling_priority_v1":    1,
			"nccl.exec_time_us":        perf.ExecTimeUS,
			"nccl.algo_bandwidth_gbps": perf.AlgoBandwidthGB,
			"nccl.bus_bandwidth_gbps":  perf.BusBandwidthGB,
			"nccl.msg_size_bytes":      float64(perf.MsgSizeBytes),
			"nccl.rank":                float64(event.Rank),
			"nccl.n_ranks":             float64(event.NRanks),
		},
		Type: ncclSpanType,
	}
}

// collTraceID derives a stable trace ID for a single collective invocation.
// All ranks in the same communicator executing seqNum share this trace ID,
// so their spans are grouped as siblings in one trace.
func collTraceID(commID string, seqNum int64) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(commID))
	_, _ = h.Write([]byte(strconv.FormatInt(seqNum, 10)))
	return h.Sum64()
}

// collSpanID derives a unique span ID for a single rank within a collective.
func collSpanID(commID string, rank int, seqNum int64) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(commID))
	_, _ = h.Write([]byte(strconv.Itoa(rank)))
	_, _ = h.Write([]byte(strconv.FormatInt(seqNum, 10)))
	return h.Sum64()
}

// extractTag returns the value portion of the first "key:value" tag matching key.
func extractTag(tags []string, key string) string {
	prefix := key + ":"
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			return t[len(prefix):]
		}
	}
	return ""
}
