// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"google.golang.org/protobuf/proto"
)

// percentile quantiles and their metric name suffixes
var percentileQuantiles = []struct {
	quantile float64
	suffix   string
}{
	{0.50, "p50"},
	{0.75, "p75"},
	{0.90, "p90"},
	{0.95, "p95"},
	{0.99, "p99"},
}

// processStatsPayload processes a StatsPayload and emits metrics via ObserveMetric.
func processStatsPayload(handle observerdef.Handle, payload *pb.StatsPayload) {
	for _, clientStats := range payload.Stats {
		for _, bucket := range clientStats.Stats {
			// Convert bucket start from nanoseconds to seconds for metric timestamp
			timestamp := float64(bucket.Start) / 1e9

			for _, gs := range bucket.Stats {
				tags := buildStatsTags(payload, clientStats, gs)

				// Emit count/duration metrics
				handle.ObserveMetric(&statsMetricView{
					name:      "trace.hits",
					value:     float64(gs.Hits),
					tags:      tags,
					timestamp: timestamp,
				})
				handle.ObserveMetric(&statsMetricView{
					name:      "trace.errors",
					value:     float64(gs.Errors),
					tags:      tags,
					timestamp: timestamp,
				})
				handle.ObserveMetric(&statsMetricView{
					name:      "trace.duration",
					value:     float64(gs.Duration),
					tags:      tags,
					timestamp: timestamp,
				})
				handle.ObserveMetric(&statsMetricView{
					name:      "trace.top_level_hits",
					value:     float64(gs.TopLevelHits),
					tags:      tags,
					timestamp: timestamp,
				})

				// Extract and emit percentiles from DDSketch for OK latencies
				if len(gs.OkSummary) > 0 {
					emitPercentiles(handle, "trace.latency.ok", gs.OkSummary, tags, timestamp)
				}

				// Extract and emit percentiles from DDSketch for error latencies
				if len(gs.ErrorSummary) > 0 {
					emitPercentiles(handle, "trace.latency.error", gs.ErrorSummary, tags, timestamp)
				}
			}
		}
	}
}

// emitPercentiles extracts percentiles from a DDSketch and emits them as metrics.
func emitPercentiles(handle observerdef.Handle, metricPrefix string, sketchBytes []byte, tags []string, timestamp float64) {
	sketch, err := decodeSketch(sketchBytes)
	if err != nil {
		pkglog.Debugf("[observer] failed to decode ddsketch: %v", err)
		return
	}
	if sketch == nil {
		return
	}

	for _, pq := range percentileQuantiles {
		value, err := sketch.GetValueAtQuantile(pq.quantile)
		if err != nil {
			pkglog.Debugf("[observer] failed to get quantile %f: %v", pq.quantile, err)
			continue
		}

		metricName := fmt.Sprintf("%s.%s", metricPrefix, pq.suffix)
		handle.ObserveMetric(&statsMetricView{
			name:      metricName,
			value:     value,
			tags:      tags,
			timestamp: timestamp,
		})
	}
}

// decodeSketch decodes a protobuf-encoded DDSketch.
func decodeSketch(data []byte) (*ddsketch.DDSketch, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var sketch sketchpb.DDSketch
	if err := proto.Unmarshal(data, &sketch); err != nil {
		return nil, err
	}

	return ddsketch.FromProto(&sketch)
}

// buildStatsTags builds the tag list for a stats metric.
func buildStatsTags(payload *pb.StatsPayload, client *pb.ClientStatsPayload, gs *pb.ClientGroupedStats) []string {
	tags := make([]string, 0, 12)

	// Core grouping dimensions
	if gs.Service != "" {
		tags = append(tags, fmt.Sprintf("service:%s", gs.Service))
	}
	if gs.Name != "" {
		tags = append(tags, fmt.Sprintf("operation:%s", gs.Name))
	}
	if gs.Resource != "" {
		tags = append(tags, fmt.Sprintf("resource:%s", gs.Resource))
	}
	if gs.Type != "" {
		tags = append(tags, fmt.Sprintf("type:%s", gs.Type))
	}

	// Client-level tags
	if client.Env != "" {
		tags = append(tags, fmt.Sprintf("env:%s", client.Env))
	}
	if client.Hostname != "" {
		tags = append(tags, fmt.Sprintf("hostname:%s", client.Hostname))
	}
	if client.Version != "" {
		tags = append(tags, fmt.Sprintf("version:%s", client.Version))
	}
	if client.ContainerID != "" {
		tags = append(tags, fmt.Sprintf("container_id:%s", client.ContainerID))
	}

	// Agent-level tags
	if payload.AgentHostname != "" && payload.AgentHostname != client.Hostname {
		tags = append(tags, fmt.Sprintf("agent_hostname:%s", payload.AgentHostname))
	}
	if payload.AgentEnv != "" && payload.AgentEnv != client.Env {
		tags = append(tags, fmt.Sprintf("agent_env:%s", payload.AgentEnv))
	}

	// Additional dimensions from grouped stats
	if gs.HTTPStatusCode > 0 {
		tags = append(tags, fmt.Sprintf("http_status_code:%d", gs.HTTPStatusCode))
	}
	if gs.SpanKind != "" {
		tags = append(tags, fmt.Sprintf("span_kind:%s", gs.SpanKind))
	}

	// is_trace_root dimension
	switch gs.IsTraceRoot {
	case pb.Trilean_TRUE:
		tags = append(tags, "is_trace_root:true")
	case pb.Trilean_FALSE:
		tags = append(tags, "is_trace_root:false")
	}

	return tags
}

// statsMetricView implements the MetricView interface for stats-derived metrics.
type statsMetricView struct {
	name      string
	value     float64
	tags      []string
	timestamp float64
}

func (m *statsMetricView) GetName() string        { return m.name }
func (m *statsMetricView) GetValue() float64      { return m.value }
func (m *statsMetricView) GetRawTags() []string   { return m.tags }
func (m *statsMetricView) GetTimestamp() float64  { return m.timestamp }
func (m *statsMetricView) GetSampleRate() float64 { return 1.0 }
