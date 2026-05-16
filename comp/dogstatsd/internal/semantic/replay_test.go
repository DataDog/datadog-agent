// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantic

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/lookback"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/seriesstats"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMilestone8SemanticReplayReproducesDebugAndLookbackViews(t *testing.T) {
	now := time.Unix(100, 0)
	records := []Record{
		testRecord(now.Add(-2*time.Second), 1, "alpha", []string{"env:prod"}, "alpha|env:prod", "udp", "origin-a"),
		testRecord(now.Add(-1*time.Second), 1, "alpha", []string{"env:prod"}, "alpha|env:prod", "udp", "origin-a"),
		testRecord(now, 2, "beta", []string{"env:dev"}, "beta|env:dev", "uds", "origin-b"),
	}

	original := newTestProjection()
	for _, record := range records {
		require.NoError(t, original.ObserveSemantic(record))
	}

	replayed := newTestProjection()
	require.NoError(t, Replay(Corpus{Version: CorpusVersion, Records: records}, replayed))

	assert.Equal(t, original.SeriesStats.Snapshot(now), replayed.SeriesStats.Snapshot(now))
	assert.Equal(t, original.Lookback.TopSeries(now, 5*time.Second, 10), replayed.Lookback.TopSeries(now, 5*time.Second, 10))
	originalByListener, err := original.Lookback.CountBy(now, 5*time.Second, lookback.GroupByListener, 10)
	require.NoError(t, err)
	replayedByListener, err := replayed.Lookback.CountBy(now, 5*time.Second, lookback.GroupByListener, 10)
	require.NoError(t, err)
	assert.Equal(t, originalByListener, replayedByListener)
}

func TestMilestone8SemanticReplayIgnoresChangedEnrichmentState(t *testing.T) {
	raw := RawMetric{
		Name:       "requests.count",
		Tags:       []string{"client:go"},
		Origin:     "pod-a",
		ListenerID: "uds",
		Type:       metrics.CountType,
		Timestamp:  time.Unix(100, 0),
		Value:      1,
	}

	captured := BuildRecord(raw, mutableEnricher{originTags: map[string][]string{"pod-a": {"env:original"}}})
	rawReplayWithChangedState := BuildRecord(raw, mutableEnricher{originTags: map[string][]string{"pod-a": {"env:changed"}}})

	semanticProjection := newTestProjection()
	require.NoError(t, Replay(Corpus{Version: CorpusVersion, Records: []Record{captured}}, semanticProjection))

	rawProjection := newTestProjection()
	require.NoError(t, rawProjection.ObserveSemantic(rawReplayWithChangedState))

	semanticTop := semanticProjection.Lookback.TopSeries(raw.Timestamp, time.Second, 1)
	rawTop := rawProjection.Lookback.TopSeries(raw.Timestamp, time.Second, 1)
	require.Len(t, semanticTop, 1)
	require.Len(t, rawTop, 1)
	assert.Equal(t, "requests.count|client:go,env:original", semanticTop[0].DebugViewKey)
	assert.Equal(t, "requests.count|client:go,env:changed", rawTop[0].DebugViewKey)
	assert.NotEqual(t, semanticTop[0].DebugViewKey, rawTop[0].DebugViewKey)
}

func TestMilestone8RawReplayStillBuildsRecordsFromCurrentEnrichment(t *testing.T) {
	raw := RawMetric{Name: "metric", Tags: []string{"client:yes"}, Origin: "pod-a", ListenerID: "udp", Timestamp: time.Unix(100, 0)}
	enricher := mutableEnricher{originTags: map[string][]string{"pod-a": {"env:current"}}}

	record := BuildRecord(raw, enricher)

	assert.Equal(t, []string{"client:yes", "env:current"}, record.Descriptor.Tags)
	assert.Equal(t, "metric|client:yes,env:current", record.Descriptor.DebugViewKey)
}

func BenchmarkMilestone8SemanticReplayProjection(b *testing.B) {
	now := time.Unix(100, 0)
	records := make([]Record, 1024)
	for i := range records {
		records[i] = testRecord(now.Add(time.Duration(i%60)*time.Second), ckey.ContextKey(i%256), fmt.Sprintf("metric-%d", i%64), []string{fmt.Sprintf("env:%d", i%4)}, fmt.Sprintf("metric-%d|env:%d", i%64, i%4), "udp", fmt.Sprintf("origin-%d", i%32))
	}
	corpus := Corpus{Version: CorpusVersion, Records: records}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		projection := newTestProjection()
		_ = Replay(corpus, projection)
	}
}

func newTestProjection() *Projection {
	return NewProjection(
		seriesstats.NewStore(seriesstats.Options{ShardCount: 4, MaxContexts: 1024, TTL: time.Minute}),
		lookback.NewStore(lookback.Options{ShardCount: 4, Window: time.Minute, BucketWidth: time.Second, MaxContextsPerBucket: 1024, MaxResults: 100}),
	)
}

func testRecord(timestamp time.Time, key ckey.ContextKey, name string, tags []string, debugViewKey string, listenerID string, origin string) Record {
	return Record{
		Descriptor: Descriptor{
			Key:          key,
			Name:         name,
			Tags:         tags,
			DebugViewKey: debugViewKey,
			ListenerID:   listenerID,
			Origin:       origin,
		},
		Type:      metrics.CountType,
		Timestamp: timestamp,
		Value:     1,
	}
}

type mutableEnricher struct {
	originTags map[string][]string
}

func (e mutableEnricher) Descriptor(raw RawMetric) Descriptor {
	tags := append([]string(nil), raw.Tags...)
	tags = append(tags, e.originTags[raw.Origin]...)
	debugViewKey := raw.Name
	if len(tags) > 0 {
		debugViewKey += "|" + joinTags(tags)
	}
	return Descriptor{
		Key:          ckey.ContextKey(len(debugViewKey)),
		Name:         raw.Name,
		Host:         raw.Host,
		Tags:         tags,
		DebugViewKey: debugViewKey,
		ListenerID:   raw.ListenerID,
		Origin:       raw.Origin,
	}
}

func joinTags(tags []string) string {
	out := ""
	for i, tag := range tags {
		if i > 0 {
			out += ","
		}
		out += tag
	}
	return out
}
