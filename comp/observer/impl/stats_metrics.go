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

// processStatsView processes a TraceStatsView and emits derived metrics via ObserveMetric.
func processStatsView(handle observerdef.Handle, stats observerdef.TraceStatsView) {
	agentHostname := stats.GetAgentHostname()
	agentEnv := stats.GetAgentEnv()

	rows := stats.GetRows()
	for rows.Next() {
		row := rows.Row()
		// Convert bucket start from nanoseconds to seconds for metric timestamp
		timestamp := float64(row.GetBucketStart()) / 1e9
		tags := buildStatsTagsFromRow(agentHostname, agentEnv, row)

		handle.ObserveMetric(&statsMetricView{name: "trace.hits", value: float64(row.GetHits()), tags: tags, timestamp: timestamp})
		handle.ObserveMetric(&statsMetricView{name: "trace.errors", value: float64(row.GetErrors()), tags: tags, timestamp: timestamp})
		handle.ObserveMetric(&statsMetricView{name: "trace.duration", value: float64(row.GetDuration()), tags: tags, timestamp: timestamp})
		handle.ObserveMetric(&statsMetricView{name: "trace.top_level_hits", value: float64(row.GetTopLevelHits()), tags: tags, timestamp: timestamp})

		if okSummary := row.GetOkSummary(); len(okSummary) > 0 {
			emitPercentiles(handle, "trace.latency.ok", okSummary, tags, timestamp)
		}
		if errSummary := row.GetErrorSummary(); len(errSummary) > 0 {
			emitPercentiles(handle, "trace.latency.error", errSummary, tags, timestamp)
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

// buildStatsTagsFromRow builds the tag list for a stat metric row.
func buildStatsTagsFromRow(agentHostname, agentEnv string, row observerdef.TraceStatRow) []string {
	tags := make([]string, 0, 12)

	if row.GetService() != "" {
		tags = append(tags, "service:"+row.GetService())
	}
	if row.GetName() != "" {
		tags = append(tags, "operation:"+row.GetName())
	}
	if row.GetResource() != "" {
		tags = append(tags, "resource:"+row.GetResource())
	}
	if row.GetType() != "" {
		tags = append(tags, "type:"+row.GetType())
	}
	if row.GetClientEnv() != "" {
		tags = append(tags, "env:"+row.GetClientEnv())
	}
	if row.GetClientHostname() != "" {
		tags = append(tags, "hostname:"+row.GetClientHostname())
	}
	if row.GetClientVersion() != "" {
		tags = append(tags, "version:"+row.GetClientVersion())
	}
	if row.GetClientContainerID() != "" {
		tags = append(tags, "container_id:"+row.GetClientContainerID())
	}
	if agentHostname != "" && agentHostname != row.GetClientHostname() {
		tags = append(tags, "agent_hostname:"+agentHostname)
	}
	if agentEnv != "" && agentEnv != row.GetClientEnv() {
		tags = append(tags, "agent_env:"+agentEnv)
	}
	if row.GetHTTPStatusCode() > 0 {
		tags = append(tags, fmt.Sprintf("http_status_code:%d", row.GetHTTPStatusCode()))
	}
	if row.GetSpanKind() != "" {
		tags = append(tags, "span_kind:"+row.GetSpanKind())
	}
	switch row.GetIsTraceRoot() {
	case 1:
		tags = append(tags, "is_trace_root:true")
	case 2:
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

// statsPayloadView adapts a *pb.StatsPayload to the TraceStatsView interface.
type statsPayloadView struct {
	payload *pb.StatsPayload
}

func (v *statsPayloadView) GetAgentHostname() string { return v.payload.AgentHostname }
func (v *statsPayloadView) GetAgentEnv() string      { return v.payload.AgentEnv }
func (v *statsPayloadView) GetRows() observerdef.TraceStatsRowIterator {
	return &statsRowIterator{payload: v.payload, clientIdx: 0, bucketIdx: 0, groupIdx: -1}
}

// statsRowIterator iterates over denormalized rows of a statsPayloadView.
type statsRowIterator struct {
	payload   *pb.StatsPayload
	clientIdx int
	bucketIdx int
	groupIdx  int
	current   *statsRowView
}

func (it *statsRowIterator) Next() bool {
	for it.clientIdx < len(it.payload.Stats) {
		client := it.payload.Stats[it.clientIdx]
		for it.bucketIdx < len(client.Stats) {
			bucket := client.Stats[it.bucketIdx]
			it.groupIdx++
			if it.groupIdx < len(bucket.Stats) {
				it.current = &statsRowView{
					client: client,
					bucket: bucket,
					group:  bucket.Stats[it.groupIdx],
				}
				return true
			}
			it.bucketIdx++
			it.groupIdx = -1
		}
		it.clientIdx++
		it.bucketIdx = 0
		it.groupIdx = -1
	}
	return false
}

func (it *statsRowIterator) Row() observerdef.TraceStatRow { return it.current }

// statsRowView adapts a (ClientStatsPayload, ClientStatsBucket, ClientGroupedStats)
// triple to the TraceStatRow interface.
type statsRowView struct {
	client *pb.ClientStatsPayload
	bucket *pb.ClientStatsBucket
	group  *pb.ClientGroupedStats
}

func (r *statsRowView) GetClientHostname() string    { return r.client.Hostname }
func (r *statsRowView) GetClientEnv() string         { return r.client.Env }
func (r *statsRowView) GetClientVersion() string     { return r.client.Version }
func (r *statsRowView) GetClientContainerID() string { return r.client.ContainerID }
func (r *statsRowView) GetBucketStart() uint64       { return r.bucket.Start }
func (r *statsRowView) GetBucketDuration() uint64    { return r.bucket.Duration }
func (r *statsRowView) GetService() string           { return r.group.Service }
func (r *statsRowView) GetName() string              { return r.group.Name }
func (r *statsRowView) GetResource() string          { return r.group.Resource }
func (r *statsRowView) GetType() string              { return r.group.Type }
func (r *statsRowView) GetHTTPStatusCode() uint32    { return r.group.HTTPStatusCode }
func (r *statsRowView) GetSpanKind() string          { return r.group.SpanKind }
func (r *statsRowView) GetIsTraceRoot() int32        { return int32(r.group.IsTraceRoot) }
func (r *statsRowView) GetSynthetics() bool          { return r.group.Synthetics }
func (r *statsRowView) GetHits() uint64              { return r.group.Hits }
func (r *statsRowView) GetErrors() uint64            { return r.group.Errors }
func (r *statsRowView) GetTopLevelHits() uint64      { return r.group.TopLevelHits }
func (r *statsRowView) GetDuration() uint64          { return r.group.Duration }
func (r *statsRowView) GetOkSummary() []byte         { return r.group.OkSummary }
func (r *statsRowView) GetErrorSummary() []byte      { return r.group.ErrorSummary }
func (r *statsRowView) GetPeerTags() []string        { return r.group.PeerTags }
