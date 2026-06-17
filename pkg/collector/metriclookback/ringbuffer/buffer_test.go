// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ringbuffer

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestAppendCanonicalizesContextsAndStoresRecords(t *testing.T) {
	now := time.Unix(123, 456789000)
	buffer := New(Options{
		Capacity:   4,
		ShardCount: 1,
		Now: func() time.Time {
			return now
		},
	})

	samples := []metrics.MetricSample{
		{
			Name:            "custom.metric",
			Value:           1.25,
			Mtype:           metrics.GaugeType,
			Tags:            []string{"b:2", "a:1", "a:1"},
			Host:            "host-a",
			SampleRate:      0.5,
			Timestamp:       10.125,
			FlushFirstValue: true,
			NoIndex:         true,
			Source:          metrics.MetricSourceInternal,
			Unit:            "request",
		},
		{
			Name:       "custom.metric",
			Value:      2.5,
			Mtype:      metrics.GaugeType,
			Tags:       []string{"a:1", "b:2"},
			Host:       "host-a",
			SampleRate: 0.75,
			NoIndex:    true,
			Source:     metrics.MetricSourceInternal,
			Unit:       "request",
		},
	}

	if err := buffer.Append(context.Background(), checkid.ID("check:abc"), samples); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	samples[0].Tags[0] = "mutated:true"

	records := buffer.snapshotRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].contextID != records[1].contextID {
		t.Fatalf("expected equal context IDs for canonical tag equivalents, got %d and %d", records[0].contextID, records[1].contextID)
	}
	if records[0].timestampUnixMicro != 10_125_000 {
		t.Fatalf("expected explicit timestamp in microseconds, got %d", records[0].timestampUnixMicro)
	}
	if records[1].timestampUnixMicro != now.UnixMicro() {
		t.Fatalf("expected fallback timestamp %d, got %d", now.UnixMicro(), records[1].timestampUnixMicro)
	}
	if records[0].value != 1.25 || records[1].value != 2.5 {
		t.Fatalf("unexpected record values: %+v", records)
	}
	if records[0].sampleRate != 0.5 || records[1].sampleRate != 0.75 {
		t.Fatalf("unexpected sample rates: %+v", records)
	}
	if records[0].flags&flagFlushFirstValue == 0 {
		t.Fatal("expected first record to retain FlushFirstValue flag")
	}
	if records[1].flags != 0 {
		t.Fatalf("expected second record to have no flags, got %d", records[1].flags)
	}

	contexts := buffer.snapshotContexts()
	if len(contexts) != 1 {
		t.Fatalf("expected 1 active context, got %d", len(contexts))
	}
	metricContext := contexts[records[0].contextID]
	if metricContext.checkID != checkid.ID("check:abc") {
		t.Fatalf("unexpected check ID: %s", metricContext.checkID)
	}
	if metricContext.name != "custom.metric" || metricContext.host != "host-a" || metricContext.mtype != metrics.GaugeType {
		t.Fatalf("unexpected context: %+v", metricContext)
	}
	if !metricContext.noIndex || metricContext.source != metrics.MetricSourceInternal || metricContext.unit != "request" {
		t.Fatalf("unexpected context metadata: %+v", metricContext)
	}
	if !reflect.DeepEqual(metricContext.tags, []string{"a:1", "b:2"}) {
		t.Fatalf("expected sorted deduped tags, got %#v", metricContext.tags)
	}

	stats := buffer.Stats()
	if stats.Capacity != 4 || stats.ShardCount != 1 || stats.Records != 2 || stats.ActiveContexts != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.TotalContextsCreated != 1 || stats.AppendedSamples != 2 || stats.OverwrittenSamples != 0 {
		t.Fatalf("unexpected counters: %+v", stats)
	}
	assertInvariants(t, buffer)
}

func TestRingOverwritesOldestAndEvictsContexts(t *testing.T) {
	buffer := New(Options{Capacity: 3, ShardCount: 1})
	for i := 0; i < 4; i++ {
		if err := buffer.Append(context.Background(), checkid.ID("check:abc"), []metrics.MetricSample{{
			Name:  "metric." + string(rune('a'+i)),
			Value: float64(i + 1),
			Mtype: metrics.GaugeType,
		}}); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	records := buffer.snapshotRecords()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	for i, expectedValue := range []float64{2, 3, 4} {
		if records[i].value != expectedValue {
			t.Fatalf("record %d value = %f, want %f (records: %+v)", i, records[i].value, expectedValue, records)
		}
	}

	contexts := buffer.snapshotContexts()
	if len(contexts) != 3 {
		t.Fatalf("expected 3 active contexts, got %d", len(contexts))
	}
	for _, ctx := range contexts {
		if ctx.name == "metric.a" {
			t.Fatalf("oldest context was not evicted: %+v", contexts)
		}
	}

	stats := buffer.Stats()
	if stats.Records != 3 || stats.ActiveContexts != 3 || stats.TotalContextsCreated != 4 || stats.AppendedSamples != 4 || stats.OverwrittenSamples != 1 {
		t.Fatalf("unexpected stats after overwrite: %+v", stats)
	}
	assertInvariants(t, buffer)
}

func TestOverwriteSameContextKeepsContextActive(t *testing.T) {
	buffer := New(Options{Capacity: 2, ShardCount: 1})
	for i := 0; i < 3; i++ {
		if err := buffer.Append(context.Background(), checkid.ID("check:abc"), []metrics.MetricSample{{
			Name:  "same.metric",
			Value: float64(i + 1),
			Mtype: metrics.CountType,
			Tags:  []string{"env:test"},
		}}); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	records := buffer.snapshotRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].value != 2 || records[1].value != 3 {
		t.Fatalf("unexpected records after overwrite: %+v", records)
	}
	if records[0].contextID != records[1].contextID {
		t.Fatalf("expected retained records to share a context: %+v", records)
	}

	stats := buffer.Stats()
	if stats.ActiveContexts != 1 || stats.TotalContextsCreated != 1 || stats.OverwrittenSamples != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertInvariants(t, buffer)
}

func TestAppendHonorsCanceledContext(t *testing.T) {
	buffer := New(Options{Capacity: 2, ShardCount: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := buffer.Append(ctx, checkid.ID("check:abc"), []metrics.MetricSample{{
		Name:  "metric",
		Value: 1,
		Mtype: metrics.GaugeType,
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	stats := buffer.Stats()
	if stats.Records != 0 || stats.ActiveContexts != 0 || stats.AppendedSamples != 0 || stats.OverwrittenSamples != 0 {
		t.Fatalf("expected empty buffer after canceled append, got %+v", stats)
	}
	assertInvariants(t, buffer)
}

func TestSingleShardRingMatchesReferenceModel(t *testing.T) {
	const capacity = 5
	buffer := New(Options{Capacity: capacity, ShardCount: 1})
	var reference []float64

	for i := 0; i < 17; i++ {
		sample := metrics.MetricSample{
			Name:      "model.metric." + strconv.Itoa(i%3),
			Value:     float64(i),
			Mtype:     metrics.MetricType(i % int(metrics.NumMetricTypes)),
			Tags:      []string{"tag:" + strconv.Itoa(i%2), "stable:true"},
			Host:      "host-" + strconv.Itoa(i%4),
			Timestamp: float64(100 + i),
		}
		if err := buffer.Append(context.Background(), checkid.ID("check:model"), []metrics.MetricSample{sample}); err != nil {
			t.Fatalf("Append returned error at step %d: %v", i, err)
		}

		reference = append(reference, sample.Value)
		if len(reference) > capacity {
			reference = reference[len(reference)-capacity:]
		}

		records := buffer.snapshotRecords()
		if len(records) != len(reference) {
			t.Fatalf("step %d: got %d records, want %d", i, len(records), len(reference))
		}
		for idx, expected := range reference {
			if records[idx].value != expected {
				t.Fatalf("step %d: record %d value = %f, want %f (records: %+v)", i, idx, records[idx].value, expected, records)
			}
		}
	}

	stats := buffer.Stats()
	if stats.Records != capacity || stats.AppendedSamples != 17 || stats.OverwrittenSamples != 12 {
		t.Fatalf("unexpected final stats: %+v", stats)
	}
	assertInvariants(t, buffer)
}

func TestConcurrentAppendMaintainsCounts(t *testing.T) {
	const (
		workers          = 8
		samplesPerWorker = 64
		totalSamples     = workers * samplesPerWorker
	)
	buffer := New(Options{Capacity: totalSamples * 4, ShardCount: 4})

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < samplesPerWorker; i++ {
				if err := buffer.Append(context.Background(), checkid.ID("check:"+strconv.Itoa(worker)), []metrics.MetricSample{{
					Name:  "concurrent.metric",
					Value: float64(worker*samplesPerWorker + i),
					Mtype: metrics.GaugeType,
					Tags:  []string{"worker:" + strconv.Itoa(worker)},
				}}); err != nil {
					t.Errorf("Append returned error: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	stats := buffer.Stats()
	if stats.Records != totalSamples || stats.AppendedSamples != totalSamples || stats.OverwrittenSamples != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.ActiveContexts != workers || stats.TotalContextsCreated != workers {
		t.Fatalf("unexpected context stats: %+v", stats)
	}

	records := buffer.snapshotRecords()
	if len(records) != totalSamples {
		t.Fatalf("expected %d records, got %d", totalSamples, len(records))
	}
	for i := 1; i < len(records); i++ {
		if records[i-1].sequence >= records[i].sequence {
			t.Fatalf("records are not globally sequence ordered around %d: %+v then %+v", i, records[i-1], records[i])
		}
	}
	assertInvariants(t, buffer)
}

func TestAppendReturnsContextErrorAfterMidBatchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	buffer := New(Options{
		Capacity:   4,
		ShardCount: 1,
		Now: func() time.Time {
			cancel()
			return time.Unix(100, 0)
		},
	})

	err := buffer.Append(ctx, checkid.ID("check:abc"), []metrics.MetricSample{
		{Name: "first", Value: 1, Mtype: metrics.GaugeType},
		{Name: "second", Value: 2, Mtype: metrics.GaugeType},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	records := buffer.snapshotRecords()
	if len(records) != 1 || records[0].value != 1 {
		t.Fatalf("expected first sample to be retained before cancellation, got %+v", records)
	}
	stats := buffer.Stats()
	if stats.Records != 1 || stats.AppendedSamples != 1 || stats.ActiveContexts != 1 {
		t.Fatalf("unexpected stats after mid-batch cancellation: %+v", stats)
	}
	assertInvariants(t, buffer)
}

func TestShardLocalCapacityAndInvariantAccounting(t *testing.T) {
	buffer := New(Options{Capacity: 6, ShardCount: 3})

	for shardID, values := range [][]float64{{1, 2, 3}, {10}, {20, 21}} {
		for _, value := range values {
			sample := sampleForShard(t, checkid.ID("check:sharded"), shardID)
			sample.Value = value
			if err := buffer.Append(context.Background(), checkid.ID("check:sharded"), []metrics.MetricSample{sample}); err != nil {
				t.Fatalf("Append returned error: %v", err)
			}
		}
	}

	stats := buffer.Stats()
	if stats.Capacity != 6 || stats.Records != 5 || stats.AppendedSamples != 6 || stats.OverwrittenSamples != 1 {
		t.Fatalf("unexpected sharded stats: %+v", stats)
	}

	contexts := buffer.snapshotContexts()
	recordsByShard := make(map[int]int)
	for _, rec := range buffer.snapshotRecords() {
		ctx, found := contexts[rec.contextID]
		if !found {
			t.Fatalf("record references missing context %d", rec.contextID)
		}
		recordsByShard[ctx.shardID]++
		if ctx.shardID == 0 && rec.value == 1 {
			t.Fatal("expected oldest shard 0 record to be overwritten")
		}
	}
	if !reflect.DeepEqual(recordsByShard, map[int]int{0: 2, 1: 1, 2: 2}) {
		t.Fatalf("unexpected per-shard record counts: %+v", recordsByShard)
	}
	assertInvariants(t, buffer)
}

func TestLookbackSenderCommitAppendsFormattedSamples(t *testing.T) {
	const formattedTimestamp = 123.456789
	buffer := New(Options{Capacity: 8, ShardCount: 1})
	checkID := checkid.ID("cpu:instance-hash")
	manager := lookbacksender.NewSenderManager(context.Background(), "default-host", buffer, func() float64 {
		return formattedTimestamp
	})

	sender, err := manager.GetSender(checkID)
	if err != nil {
		t.Fatalf("GetSender returned error: %v", err)
	}
	sender.SetCheckCustomTags([]string{"check_tag:one"})
	sender.SetCheckService("svc")
	sender.FinalizeCheckServiceTag()
	sender.SetNoIndex(true)

	sender.Gauge("lookback.gauge", 1, "", []string{"b:2", "a:1"})
	if err := sender.CountWithTimestamp("lookback.count_ts", 2, "explicit-host", []string{"z:9"}, 42.25); err != nil {
		t.Fatalf("CountWithTimestamp returned error: %v", err)
	}
	sender.Commit()

	records := buffer.snapshotRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 ringbuffer records, got %d", len(records))
	}
	if records[0].value != 1 || records[0].timestampUnixMicro != 123_456_789 {
		t.Fatalf("unexpected formatted gauge record: %+v", records[0])
	}
	if records[1].value != 2 || records[1].timestampUnixMicro != 42_250_000 {
		t.Fatalf("unexpected timestamped count record: %+v", records[1])
	}

	contexts := buffer.snapshotContexts()
	gaugeContext := contexts[records[0].contextID]
	if gaugeContext.name != "lookback.gauge" || gaugeContext.host != "default-host" || gaugeContext.mtype != metrics.GaugeType {
		t.Fatalf("unexpected gauge context: %+v", gaugeContext)
	}
	if !gaugeContext.noIndex || gaugeContext.source != metrics.CheckNameToMetricSource("cpu") {
		t.Fatalf("unexpected gauge context metadata: %+v", gaugeContext)
	}
	if !reflect.DeepEqual(gaugeContext.tags, []string{"a:1", "b:2", "check_tag:one", "service:svc"}) {
		t.Fatalf("unexpected gauge tags: %#v", gaugeContext.tags)
	}

	countContext := contexts[records[1].contextID]
	if countContext.name != "lookback.count_ts" || countContext.host != "explicit-host" || countContext.mtype != metrics.CountWithTimestampType {
		t.Fatalf("unexpected count context: %+v", countContext)
	}
	if !countContext.noIndex || countContext.source != metrics.CheckNameToMetricSource("cpu") {
		t.Fatalf("unexpected count context metadata: %+v", countContext)
	}
	if !reflect.DeepEqual(countContext.tags, []string{"check_tag:one", "service:svc", "z:9"}) {
		t.Fatalf("unexpected count tags: %#v", countContext.tags)
	}

	stats := buffer.Stats()
	if stats.Records != 2 || stats.ActiveContexts != 2 || stats.AppendedSamples != 2 || stats.OverwrittenSamples != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertInvariants(t, buffer)
}

func TestSeriesExportReconstructsRetainedSamples(t *testing.T) {
	now := time.Unix(200, 0)
	buffer := New(Options{Capacity: 8, ShardCount: 1, Now: func() time.Time { return now }})
	checkID := checkid.ID("export:check")

	appendSample := func(name string, value float64, mtype metrics.MetricType, ts float64, tags []string) {
		if err := buffer.Append(context.Background(), checkID, []metrics.MetricSample{{
			Name:      name,
			Value:     value,
			Mtype:     mtype,
			Tags:      tags,
			Host:      "host-x",
			Timestamp: ts,
			NoIndex:   true,
			Source:    metrics.MetricSourceInternal,
			Unit:      "req",
		}}); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	appendSample("lookback.gauge", 1.5, metrics.GaugeType, 10, []string{"b:2", "a:1"})
	appendSample("lookback.count", 7, metrics.CountType, 0, []string{"c:3"}) // ts 0 -> fallback to now
	appendSample("lookback.rate", 3, metrics.RateType, 30, nil)

	series := buffer.Series()
	if len(series) != 3 {
		t.Fatalf("expected 3 series, got %d", len(series))
	}

	// Ordering must follow append sequence.
	if series[0].Name != "lookback.gauge" || series[1].Name != "lookback.count" || series[2].Name != "lookback.rate" {
		t.Fatalf("unexpected series order: %s, %s, %s", series[0].Name, series[1].Name, series[2].Name)
	}

	// Gauge serie: point, type mapping, context metadata, canonical tags.
	gauge := series[0]
	if len(gauge.Points) != 1 || gauge.Points[0].Value != 1.5 || gauge.Points[0].Ts != 10 {
		t.Fatalf("unexpected gauge point: %+v", gauge.Points)
	}
	if gauge.MType != metrics.APIGaugeType {
		t.Fatalf("expected APIGaugeType, got %v", gauge.MType)
	}
	if gauge.Host != "host-x" || !gauge.NoIndex || gauge.Unit != "req" || gauge.Source != metrics.MetricSourceInternal || gauge.SourceTypeName != "System" {
		t.Fatalf("unexpected gauge context metadata: %+v", gauge)
	}
	if tags := gauge.Tags.UnsafeToReadOnlySliceString(); !reflect.DeepEqual(tags, []string{"a:1", "b:2"}) {
		t.Fatalf("expected sorted canonical tags, got %#v", tags)
	}

	// Count serie: timestamp falls back to now and remains count-like because
	// Count submissions already represent a scalar count for the check run.
	count := series[1]
	if count.MType != metrics.APICountType {
		t.Fatalf("expected APICountType, got %v", count.MType)
	}
	if count.Points[0].Ts != float64(now.UnixMicro())/1e6 || count.Points[0].Value != 7 {
		t.Fatalf("unexpected count point: %+v", count.Points)
	}

	// Rate serie stores the submitted scalar without computing a backend rate.
	if series[2].MType != metrics.APIGaugeType {
		t.Fatalf("expected APIGaugeType, got %v", series[2].MType)
	}

	// Export is non-destructive: the buffer still holds the samples.
	if stats := buffer.Stats(); stats.Records != 3 {
		t.Fatalf("expected export to be non-destructive, got %d records", stats.Records)
	}

	// SerieSource iterates the same series and reports the count.
	source := buffer.SerieSource()
	if source.Count() != 3 {
		t.Fatalf("expected SerieSource count 3, got %d", source.Count())
	}
	var iterated int
	for source.MoveNext() {
		if source.Current() == nil {
			t.Fatal("SerieSource returned nil serie")
		}
		iterated++
	}
	if iterated != 3 {
		t.Fatalf("expected to iterate 3 series, got %d", iterated)
	}
}

func TestSeriesBetweenFiltersByTimestampWindow(t *testing.T) {
	buffer := New(Options{Capacity: 8, ShardCount: 1})
	checkID := checkid.ID("window:check")

	for _, sample := range []metrics.MetricSample{
		{Name: "before", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10},
		{Name: "start", Value: 2, Mtype: metrics.GaugeType, Timestamp: 20},
		{Name: "middle", Value: 3, Mtype: metrics.GaugeType, Timestamp: 25},
		{Name: "end", Value: 4, Mtype: metrics.GaugeType, Timestamp: 30},
		{Name: "after", Value: 5, Mtype: metrics.GaugeType, Timestamp: 40},
	} {
		if err := buffer.Append(context.Background(), checkID, []metrics.MetricSample{sample}); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	series := buffer.SeriesBetween(time.Unix(20, 0), time.Unix(30, 0))
	if len(series) != 3 {
		t.Fatalf("expected 3 series in window, got %d", len(series))
	}
	for i, want := range []string{"start", "middle", "end"} {
		if series[i].Name != want {
			t.Fatalf("series[%d].Name = %q, want %q", i, series[i].Name, want)
		}
	}

	if got := buffer.SerieSourceBetween(time.Time{}, time.Unix(20, 0)).Count(); got != 2 {
		t.Fatalf("expected unbounded-start source count 2, got %d", got)
	}
	if got := buffer.SerieSourceBetween(time.Unix(30, 0), time.Time{}).Count(); got != 2 {
		t.Fatalf("expected unbounded-end source count 2, got %d", got)
	}
	if series := buffer.SeriesBetween(time.Unix(31, 0), time.Unix(30, 0)); series != nil {
		t.Fatalf("expected nil series for invalid range, got %#v", series)
	}
}

func TestAPIMetricTypeMatchesRawLookbackPlan(t *testing.T) {
	tests := []struct {
		name string
		in   metrics.MetricType
		want metrics.APIMetricType
	}{
		{name: "gauge", in: metrics.GaugeType, want: metrics.APIGaugeType},
		{name: "gauge_with_timestamp", in: metrics.GaugeWithTimestampType, want: metrics.APIGaugeType},
		{name: "rate", in: metrics.RateType, want: metrics.APIGaugeType},
		{name: "count", in: metrics.CountType, want: metrics.APICountType},
		{name: "counter", in: metrics.CounterType, want: metrics.APICountType},
		{name: "monotonic_count", in: metrics.MonotonicCountType, want: metrics.APIGaugeType},
		{name: "count_with_timestamp", in: metrics.CountWithTimestampType, want: metrics.APICountType},
		{name: "histogram", in: metrics.HistogramType, want: metrics.APIGaugeType},
		{name: "historate", in: metrics.HistorateType, want: metrics.APIGaugeType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apiMetricType(tt.in); got != tt.want {
				t.Fatalf("apiMetricType(%s) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestSeriesExportEmptyBuffer(t *testing.T) {
	buffer := New(Options{Capacity: 4, ShardCount: 2})
	if series := buffer.Series(); series != nil {
		t.Fatalf("expected nil series for empty buffer, got %#v", series)
	}
	if source := buffer.SerieSource(); source.Count() != 0 || source.MoveNext() {
		t.Fatal("expected empty SerieSource")
	}
}

func BenchmarkAppendStableContext(b *testing.B) {
	buffer := New(Options{Capacity: 1024, ShardCount: 4})
	sample := metrics.MetricSample{
		Name:  "benchmark.metric",
		Value: 1,
		Mtype: metrics.GaugeType,
		Tags:  []string{"env:bench", "region:local"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := buffer.Append(context.Background(), checkid.ID("check:bench"), []metrics.MetricSample{sample}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAppendHighCardinality(b *testing.B) {
	buffer := New(Options{Capacity: 1024, ShardCount: 4})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := buffer.Append(context.Background(), checkid.ID("check:bench"), []metrics.MetricSample{{
			Name:  "benchmark.metric." + strconv.Itoa(i),
			Value: float64(i),
			Mtype: metrics.GaugeType,
			Tags:  []string{"env:bench", "iteration:" + strconv.Itoa(i)},
		}}); err != nil {
			b.Fatal(err)
		}
	}
}

func assertInvariants(t *testing.T, buffer *Buffer) {
	t.Helper()

	records := buffer.snapshotRecords()
	refs := buffer.contexts.snapshotContextRefs()
	counts := make(map[uint64]int)
	for _, rec := range records {
		if rec.contextID == 0 {
			t.Fatalf("record has zero context ID: %+v", rec)
		}
		counts[rec.contextID]++
		if _, found := refs[rec.contextID]; !found {
			t.Fatalf("record references missing context %d", rec.contextID)
		}
	}

	for contextID, count := range counts {
		if refs[contextID] != count {
			t.Fatalf("context %d refcount = %d, want retained record count %d", contextID, refs[contextID], count)
		}
	}
	for contextID, refs := range refs {
		if counts[contextID] != refs {
			t.Fatalf("context %d has stale refcount %d with retained record count %d", contextID, refs, counts[contextID])
		}
	}

	stats := buffer.Stats()
	if stats.Records != len(records) {
		t.Fatalf("Stats.Records = %d, want %d", stats.Records, len(records))
	}
	if stats.ActiveContexts != len(refs) {
		t.Fatalf("Stats.ActiveContexts = %d, want %d", stats.ActiveContexts, len(refs))
	}
}

func sampleForShard(t *testing.T, checkID checkid.ID, shardID int) metrics.MetricSample {
	t.Helper()
	for i := 0; i < 10_000; i++ {
		sample := metrics.MetricSample{
			Name:  "sharded.metric." + strconv.Itoa(shardID) + "." + strconv.Itoa(i),
			Mtype: metrics.GaugeType,
			Tags:  []string{"target_shard:" + strconv.Itoa(shardID)},
		}
		key := buildContextKey(checkID, sample, canonicalTags(sample.Tags))
		if shardIndex(hashString(key), 3) == shardID {
			return sample
		}
	}
	t.Fatalf("could not find sample for shard %d", shardID)
	return metrics.MetricSample{}
}

func (b *Buffer) snapshotRecords() []record {
	if b == nil {
		return nil
	}

	var out []record
	for i := range b.shards {
		out = append(out, b.shards[i].snapshotRecords()...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].sequence < out[j].sequence
	})
	return out
}

func (s *shard) snapshotRecords() []record {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]record, 0, s.length)
	for i := 0; i < s.length; i++ {
		idx := (s.start + i) % len(s.records)
		out = append(out, s.records[idx])
	}
	return out
}

func (b *Buffer) snapshotContexts() map[uint64]metricContext {
	if b == nil {
		return nil
	}
	return b.contexts.snapshotContexts()
}

func (s *contextStore) snapshotContexts() map[uint64]metricContext {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[uint64]metricContext, len(s.byID))
	for id, entry := range s.byID {
		ctx := entry.ctx
		ctx.tags = append([]string(nil), entry.ctx.tags...)
		out[id] = ctx
	}
	return out
}

func (s *contextStore) snapshotContextRefs() map[uint64]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[uint64]int, len(s.byID))
	for id, entry := range s.byID {
		out[id] = entry.refs
	}
	return out
}
