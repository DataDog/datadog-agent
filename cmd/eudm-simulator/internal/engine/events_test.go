// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/eventlog"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

type replayEventLines chan string

func (w replayEventLines) Write(data []byte) (int, error) {
	w <- string(data)
	return len(data), nil
}

func TestReplayEventsWaitForConfirmedDelivery(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "accepted"
		if failed {
			name = "failed"
		}
		t.Run(name, func(t *testing.T) {
			request := progressRequest(t)
			prepared, err := prepare(request)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			gate := make(chan struct{})
			delivery := &progressDelivery{recordingDelivery: &recordingDelivery{start: request.Plan.Start}, sendGate: gate, sendEntered: make(chan schema.Stream, 1)}
			if failed {
				delivery.failAt = 1
			}
			lines := make(replayEventLines, 8)
			events := eventlog.New(lines)
			d := prepared.devices[0]
			timeline := d.timelines[schema.Metrics]
			_, offset := timeline.at(0)
			done := make(chan error, 1)
			go func() {
				done <- prepared.deliver(ctx, delivery, job{device: d, timeline: timeline, stream: schema.Metrics, offset: offset}, events)
			}()
			receiveProgressValue(ctx, t, delivery.sendEntered)
			// Flush earlier messages through the logger: a premature delivery
			// event would appear before this barrier while Send is still blocked.
			events.Log("test", "delivery is blocked")
			if line := receiveProgressValue(ctx, t, lines); !strings.Contains(line, "[test] delivery is blocked") {
				t.Fatal("queued telemetry was reported as delivered", line)
			}
			close(gate)
			err = receiveProgressValue(ctx, t, done)
			if (err != nil) != failed {
				t.Fatalf("delivery result changed: %v", err)
			}
			events.Close("replay", "finished")
			line := receiveProgressValue(ctx, t, lines)
			want := "[metrics] delivered "
			if failed {
				want = "[metrics] delivery failed;"
			}
			if !strings.Contains(line, want) || !strings.Contains(line, "device="+d.id.Hostname) || !strings.Contains(line, "phase=healthy; cycle=1") {
				t.Fatal("missing confirmed result or simulated device context", line)
			}
			at, _, _ := strings.Cut(line, " ")
			if _, err := time.Parse(time.RFC3339, at); err != nil {
				t.Fatal("missing event timestamp", line)
			}
			if line = receiveProgressValue(ctx, t, lines); !strings.Contains(line, "[replay] finished") || len(lines) != 0 {
				t.Fatal("completion preceded delivery events or duplicated a cycle", line)
			}
		})
	}
}

func TestReplayEventsMatchDeliveredCyclesWithoutSummaries(t *testing.T) {
	request := progressRequest(t)
	var output bytes.Buffer
	events := eventlog.New(&output)
	result, err := Run(context.Background(), request, Options{Workers: 2, QueueCapacity: 1, Clock: &advancingClock{now: request.Plan.Start}, Delivery: &recordingDelivery{start: request.Plan.Start}, Events: events})
	events.Close("replay", "finished")
	if err != nil || !result.Complete() {
		t.Fatalf("event output changed replay: %v", err)
	}
	got := output.String()
	for stream, counts := range result.Ledger[0].Streams {
		if count := strings.Count(got, "["+string(stream)+"] delivered "); uint64(count) != counts.Delivered {
			t.Fatalf("%s: %d logged deliveries, %d confirmed", stream, count, counts.Delivered)
		}
	}
	for _, want := range []string{"[replay] started; devices=1", "dispatching phase=healthy", "waiting for deliveries", "draining Agent delivery", " series)", " applications)", " chunks)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	if strings.Count(got, "[replay] started;") != 1 || strings.Contains(got, "delivered cycles:") || strings.Contains(got, "Replay 0s/") || strings.Contains(got, "omitted") {
		t.Fatal("duplicate startup, periodic summaries, or missing events", got)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		at, _, _ := strings.Cut(line, " ")
		if _, err := time.Parse(time.RFC3339, at); err != nil {
			t.Fatal("missing timestamp", line)
		}
	}
}

func TestReplayNetworkEvents(t *testing.T) {
	request := shippedRequest(t, "wifi-degradation-macos")
	for i := range request.Scenario.Fleet {
		request.Scenario.Fleet[i].Count = 1
	}
	request = requestFor(t, request.Scenario, request.Plan.ScenarioDigest, request.Bundle)
	prepared, err := prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	events := eventlog.New(&output)
	delivery := &recordingDelivery{start: request.Plan.Start}
	for _, stream := range []schema.Stream{APMetricStream, NDMStream} {
		if err := prepared.deliver(context.Background(), delivery, job{stream: stream}, events); err != nil {
			t.Fatal(err)
		}
	}
	events.Close("replay", "finished")
	for _, want := range []string{"[access_point_metrics] delivered access-point metrics (", "[ndm_metadata] delivered metadata (", " batches)", "network devices; phase=healthy; cycle=1"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q in %s", want, output.String())
		}
	}
}
