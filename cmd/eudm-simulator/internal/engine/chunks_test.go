// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestReplayPreservesEmptyChunksWithinCompleteGroups(t *testing.T) {
	// Supply verified in-memory bytes independently of the on-disk fixtures.
	// Both complete groups have an empty first chunk and a populated second one.
	b := &bundle.Loaded{
		Digest: schema.Digest([]byte("complete-groups-with-empty-chunks")),
		Files:  map[string][]byte{},
		Manifest: bundle.Manifest{
			CaptureTool: bundle.BuildIdentity{Version: "7.85.0", Commit: fixtureCommit},
			Profile: schema.Profile{OS: "windows", Architecture: "amd64", MemoryBytes: 8 << 30,
				Streams: []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Connections, schema.Software, schema.AgentInventory, schema.HostInventory}},
			Files:          map[string]string{},
			Cadences:       map[schema.Stream]time.Duration{},
			MetricCadences: map[string]time.Duration{"cpu": 10 * time.Second},
		},
	}
	add := func(stream schema.Stream, chunk, count int, value any) {
		t.Helper()
		data, err := telemetry.Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("%s-%d.json", stream, chunk)
		b.Files[name], b.Manifest.Files[name] = data, schema.Digest(data)
		b.Manifest.Cadences[stream] = 10 * time.Second
		b.Manifest.Samples = append(b.Manifest.Samples, bundle.SampleRef{
			Stream: stream, ProducerID: string(stream), CycleID: 1, Sequence: 1,
			ChunkIndex: chunk, ChunkCount: count, File: name,
		})
	}
	series, err := telemetry.NewMetricSample([]*metrics.Serie{{Name: "system.cpu.user", Host: "capture-host", MType: metrics.APIGaugeType, Points: []metrics.Point{{Value: 5}}}})
	if err != nil {
		t.Fatal(err)
	}
	add(schema.Metrics, 0, 1, series)
	add(schema.HostMetadata, 0, 1, &telemetry.HostMetadata{Hostname: "capture-host", AgentVersion: "7.85.0", OS: "windows"})
	add(schema.AgentInventory, 0, 1, &telemetrycapture.Inventory{Hostname: "capture-host", UUID: "capture-uuid", Agent: &telemetrycapture.AgentInventoryMetadata{AgentVersion: "7.85.0", Flavor: "agent", InfrastructureMode: "end_user_device", AgentStartupTimeMS: -3000}})
	add(schema.HostInventory, 0, 1, &telemetrycapture.Inventory{Hostname: "capture-host", UUID: "capture-uuid", Host: &telemetrycapture.HostInventoryMetadata{AgentVersion: "7.85.0", OS: "windows", CPUCores: 4, CPULogicalProcessors: 8, MemoryTotalKb: (8 << 30) / 1024}})
	add(schema.Software, 0, 1, json.RawMessage(`{"hostname":"capture-host","host_software":{"software":[{"name":"OS","software_type":"os"}]}}`))
	process := &model.CollectorProc{HostName: "capture-host", GroupId: 7, GroupSize: 2, Info: &model.SystemInfo{TotalMemory: 8 << 30, Os: &model.OSInfo{Name: "windows"}}}
	add(schema.Processes, 0, 2, process)
	process.Processes = []*model.Process{{Pid: 100, Command: &model.Command{Comm: "captured-app"}}}
	add(schema.Processes, 1, 2, process)
	connections := &model.CollectorConnections{HostName: "capture-host", GroupId: 8, GroupSize: 2}
	add(schema.Connections, 0, 2, connections)
	connections.Connections = []*model.Connection{{Pid: 100, Laddr: &model.Addr{Ip: "192.0.2.1", Port: 1234}, Raddr: &model.Addr{Ip: "192.0.2.2", Port: 443}}}
	add(schema.Connections, 1, 2, connections)

	scenario := &schema.Scenario{Version: schema.Version, Meta: schema.ScenarioMeta{Name: "empty-chunk-replay"}, Expectation: schema.Expectation{Conclusion: schema.Healthy},
		Fleet:  []schema.GroupDef{{Group: "primary", OS: "windows", Count: 1}},
		Phases: []schema.Phase{{Name: "healthy", Duration: schema.Duration{Duration: 10 * time.Second}}},
	}
	request := requestFor(t, scenario, schema.Digest([]byte("empty-chunk-replay")), b)
	if err := Validate(request); err != nil {
		t.Fatalf("complete groups rejected by preparation or overlay preflight: %v", err)
	}
	delivery := &recordingDelivery{start: request.Plan.Start, retainPayload: true}
	if _, err := Run(context.Background(), request, Options{Workers: 2, QueueCapacity: 2, Clock: &advancingClock{now: request.Plan.Start}, Delivery: delivery}); err != nil {
		t.Fatalf("complete groups rejected by delivery: %v", err)
	}
	groups := 0
	for _, record := range delivery.records {
		if record.Stream != schema.Processes && record.Stream != schema.Connections {
			continue
		}
		groups++
		var chunks []json.RawMessage
		if err := json.Unmarshal([]byte(record.Payload), &chunks); err != nil || len(chunks) != 2 || record.Chunks != 2 {
			t.Fatalf("logical group was split or discarded: %+v, %v", record, err)
		}
		for i, data := range chunks {
			sample, err := telemetry.DecodeGroupChunk(record.Stream, data)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			if record.Stream == schema.Processes {
				count = len(sample.Processes.Processes)
			} else {
				count = len(sample.Connections.Connections)
			}
			if count != i {
				t.Fatalf("chunk order changed: index=%d records=%d", i, count)
			}
		}
	}
	if groups != 2 {
		t.Fatalf("delivered %d process/connection groups, want 2", groups)
	}
	for _, ref := range b.Manifest.Samples {
		if (ref.Stream == schema.Processes || ref.Stream == schema.Connections) && ref.ChunkIndex == 0 {
			ref.ChunkCount = 1
			if _, err := decodeCapturedSample(b, ref); err == nil {
				t.Fatalf("accepted empty singleton %s sample", ref.Stream)
			}
		}
	}
}
