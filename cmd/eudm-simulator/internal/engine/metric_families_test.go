// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/report"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func familyMetrics(t *testing.T, cpu, battery []time.Duration) *bundle.Loaded {
	t.Helper()
	b := &bundle.Loaded{Files: map[string][]byte{}, Manifest: bundle.Manifest{
		Files: map[string]string{}, Cadences: map[schema.Stream]time.Duration{schema.Metrics: 15 * time.Second},
		MetricCadences: map[string]time.Duration{"cpu": 15 * time.Second, "battery": 300 * time.Second},
	}}
	at := map[time.Duration][]string{}
	for _, offset := range cpu {
		at[offset] = append(at[offset], "system.cpu.user", "system.cpu.idle")
	}
	for _, offset := range battery {
		at[offset] = append(at[offset], "system.battery.current_charge_pct")
	}
	var offsets []time.Duration
	for offset := range at {
		offsets = append(offsets, offset)
	}
	slices.Sort(offsets)
	for index, offset := range offsets {
		var series []*metrics.Serie
		for _, name := range at[offset] {
			series = append(series, &metrics.Serie{Name: name, Host: "capture-host", MType: metrics.APIGaugeType,
				Points: []metrics.Point{{Ts: offset.Seconds() - .25, Value: float64(index + 1)}}, Tags: tagset.CompositeTagsFromSlice([]string{"infra_mode:end_user_device"})})
			if name == "system.cpu.idle" {
				series[len(series)-1].Points[0].Value = 90
			}
		}
		payload, err := telemetry.NewMetricSample(series)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		file := fmt.Sprintf("family-metrics-%d.json", index)
		b.Files[file], b.Manifest.Files[file] = data, schema.Digest(data)
		b.Manifest.Samples = append(b.Manifest.Samples, bundle.SampleRef{Stream: schema.Metrics, ProducerID: "core", CycleID: uint64(index + 1), Sequence: uint64(index + 1), ChunkCount: 1, Offset: offset, File: file})
	}
	return b
}

func familyRequest(t *testing.T, metricBundle *bundle.Loaded, duration time.Duration) Request {
	t.Helper()
	r := sharedBaselineRequest(t)
	r.Scenario.Fleet = r.Scenario.Fleet[:1]
	r.Scenario.Fleet[0].Count = 1
	r.Scenario.Phases[0].Duration.Duration = duration
	b := r.Bundle
	b.Manifest.Samples = slices.DeleteFunc(b.Manifest.Samples, func(ref bundle.SampleRef) bool { return ref.Stream == schema.Metrics })
	b.Manifest.Samples = append(b.Manifest.Samples, metricBundle.Manifest.Samples...)
	b.Manifest.MetricCadences = metricBundle.Manifest.MetricCadences
	b.Manifest.Profile.MetricNames = []string{"system.cpu.user", "system.cpu.idle", "system.battery.current_charge_pct"}
	for name, data := range metricBundle.Files {
		b.Files[name], b.Manifest.Files[name] = data, schema.Digest(data)
	}
	return requestFor(t, r.Scenario, schema.Digest([]byte("independent-family-cadences")), b)
}

func TestMetricFamilyTimelinePreservesSlowChecksAndSkippedCycles(t *testing.T) {
	b := familyMetrics(t, []time.Duration{10 * time.Second, 25 * time.Second, 55 * time.Second}, []time.Duration{265 * time.Second, 565 * time.Second})
	timelines, err := makeTimelines(b, schema.Metrics, 1200*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, timeline := range timelines {
		want := []time.Duration{265 * time.Second, 565 * time.Second, 865 * time.Second, 1165 * time.Second}
		if timeline.family == "cpu" {
			want = []time.Duration{10 * time.Second, 25 * time.Second, 55 * time.Second, 70 * time.Second, 85 * time.Second, 115 * time.Second}
		} else if timeline.count != int64(len(want)) {
			t.Fatalf("battery count %d, want %d", timeline.count, len(want))
		}
		for i, offset := range want {
			cycle, got := timeline.at(int64(i))
			if got != offset || len(cycle.refs) != 1 || cycle.producerID != "core" || cycle.cycleID != cycle.refs[0].CycleID {
				t.Fatalf("family %s lost timing or cycle membership at %d: %+v at %s", timeline.family, i, cycle, got)
			}
		}
	}
	delete(b.Manifest.MetricCadences, "battery")
	if _, err := makeTimelines(b, schema.Metrics, 1200*time.Second); err == nil {
		t.Fatal("silently fell back to the serializer cadence for battery")
	}
}

func TestMetricFamiliesReplayCompleteObservedSeriesWithIndependentPeriods(t *testing.T) {
	var cpu []time.Duration
	for at := 10 * time.Second; at <= 805*time.Second; at += 15 * time.Second {
		cpu = append(cpu, at)
	}
	r := familyRequest(t, familyMetrics(t, cpu, []time.Duration{265 * time.Second, 565 * time.Second}), 1200*time.Second)
	// A CPU-only collection must not require a battery reading, while all
	// observed members of the CPU family remain available to the overlay.
	r.Scenario.Phases[0].Metrics = map[string]map[string]schema.Pattern{"primary": {
		"system.cpu.user":                   {Steady: &schema.SteadyPattern{Value: 10}},
		"system.battery.current_charge_pct": {Steady: &schema.SteadyPattern{Value: 80}},
	}}
	sink := &recordingDelivery{start: r.Plan.Start, retainPayload: true}
	result, err := Run(context.Background(), r, Options{Workers: 4, QueueCapacity: 1, Clock: &advancingClock{now: r.Plan.Start}, Delivery: sink})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]time.Duration{}
	for _, record := range sink.records {
		if record.Stream != schema.Metrics {
			continue
		}
		var chunks []json.RawMessage
		if err := json.Unmarshal([]byte(record.Payload), &chunks); err != nil || len(chunks) != 1 {
			t.Fatalf("invalid family collection: %v", err)
		}
		sample, err := telemetry.Decode(schema.Metrics, chunks[0])
		if err != nil {
			t.Fatalf("mixed or missing metric family series: %v", err)
		}
		family := telemetrycapture.MetricFamily(sample.Metrics[0].Name)
		wantSeries := 1
		if family == "cpu" {
			wantSeries = 2
		}
		if len(sample.Metrics) != wantSeries {
			t.Fatal("lost series from a complete metric family collection")
		}
		got[family] = append(got[family], record.Offset)
		for _, serie := range sample.Metrics {
			if telemetrycapture.MetricFamily(serie.Name) != family || math.Abs(serie.Points[0].Ts-(record.Offset.Seconds()-.25)) > 1e-6 {
				t.Fatal("metric family filter/rebase lost collection or fractional point offset")
			}
			if !slices.Contains(serie.Tags.UnsafeToReadOnlySliceString(), "infra_mode:end_user_device") {
				t.Fatal("family partition lost tags")
			}
			expected := 90.0 // The untouched idle series must survive each family copy.
			if pattern, ok := r.Scenario.Phases[0].Metrics["primary"][serie.Name]; ok {
				expected = pattern.Steady.Value
			}
			if serie.Points[0].Value != expected {
				t.Fatal("family partition skipped an overlay")
			}
		}
	}
	var expectedCPU []time.Duration
	for at := 10 * time.Second; at < 1200*time.Second; at += 15 * time.Second {
		expectedCPU = append(expectedCPU, at)
	}
	slices.Sort(got["cpu"])
	slices.Sort(got["battery"])
	if !slices.Equal(got["cpu"], expectedCPU) || !slices.Equal(got["battery"], []time.Duration{265 * time.Second, 565 * time.Second, 865 * time.Second, 1165 * time.Second}) {
		t.Fatalf("family cadences differ: CPU=%v battery=%v", got["cpu"], got["battery"])
	}
	counts := result.Ledger[0].Streams[schema.Metrics]
	if counts.Expected != 84 || counts.Delivered != 84 || counts.Failed != 0 || !result.Complete() || result.Progress.Activity != "succeeded" {
		t.Fatalf("metric ledger must count completed family collections: %+v", counts)
	}
	for name, digest := range r.Bundle.Manifest.Files {
		if schema.Digest(r.Bundle.Files[name]) != digest {
			t.Fatal("family filtering mutated captured evidence")
		}
	}
}

type familyGateDelivery struct {
	*recordingDelivery
	muFamily                   sync.Mutex
	cpuCalls                   int
	cpuGate, batteryGate       <-chan struct{}
	cpuEntered, batteryEntered chan struct{}
	batteryOnce                sync.Once
}

func (d *familyGateDelivery) Send(ctx context.Context, at time.Time, stream schema.Stream, samples []*telemetry.Sample) error {
	if stream == schema.Metrics {
		family := telemetrycapture.MetricFamily(samples[0].Metrics[0].Name)
		var gate <-chan struct{}
		if family == "battery" {
			d.batteryOnce.Do(func() { close(d.batteryEntered) })
			gate = d.batteryGate
		} else {
			d.muFamily.Lock()
			d.cpuCalls++
			calls := d.cpuCalls
			d.muFamily.Unlock()
			if calls == 2 {
				close(d.cpuEntered)
				gate = d.cpuGate
			}
		}
		if gate != nil {
			select {
			case <-gate:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return d.recordingDelivery.Send(ctx, at, stream, samples)
}

func TestMetricFamilyProgressCountsDeliveryAndFamiliesRunIndependently(t *testing.T) {
	r := familyRequest(t, familyMetrics(t, []time.Duration{0, 15 * time.Second}, []time.Duration{0, 300 * time.Second}), 31*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cpuGate, batteryGate := make(chan struct{}), make(chan struct{})
	delivery := &familyGateDelivery{recordingDelivery: &recordingDelivery{start: r.Plan.Start}, cpuGate: cpuGate, batteryGate: batteryGate, cpuEntered: make(chan struct{}), batteryEntered: make(chan struct{})}
	clock := &advancingClock{now: r.Plan.Start}
	ticks := make(chan time.Time)
	snapshots := make(chan *report.Report, 4)
	done := make(chan progressResult, 1)
	go func() {
		result, err := Run(ctx, r, Options{Workers: 2, QueueCapacity: 1, Clock: clock, Delivery: delivery, progressTicks: ticks, Progress: func(r *report.Report) error { snapshots <- r; return nil }})
		done <- progressResult{result, err}
	}()
	initial := receiveProgressValue(ctx, t, snapshots)
	if initial.Ledger[0].Streams[schema.Metrics].Delivered != 0 {
		t.Fatal("initial progress contains undelivered family collections")
	}
	receiveProgressValue(ctx, t, delivery.batteryEntered)
	receiveProgressValue(ctx, t, delivery.cpuEntered)
	sendProgressTick(ctx, t, ticks, clock)
	early := receiveProgressValue(ctx, t, snapshots)
	if count := early.Ledger[0].Streams[schema.Metrics]; count.Expected != 4 || count.Delivered != 1 {
		t.Fatalf("blocked battery must not block or count as completed CPU delivery: %+v", count)
	}
	close(cpuGate)
	close(batteryGate)
	final := receiveProgressValue(ctx, t, done)
	if final.err != nil || !final.report.Complete() || final.report.Ledger[0].Streams[schema.Metrics].Delivered != 4 || early.Ledger[0].Streams[schema.Metrics].Delivered != 1 {
		t.Fatalf("family completion or owned progress snapshot failed: %v", final.err)
	}
}
