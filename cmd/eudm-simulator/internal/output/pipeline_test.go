// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib

package output

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/impl"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestPortableNativeBundleRoundTripThroughAgentDelivery(t *testing.T) {
	for _, platform := range []string{"windows", "macos"} {
		t.Run(platform, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			destinations, err := (safety.Config{Site: safety.Site}).Resolve(func(string) string { return "" })
			if err != nil {
				t.Fatal(err)
			}
			recorder := NewRecorder()
			p, err := New(ctx, destinations, "recording-only-no-credential", recorder)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			normalizer := capture.NewNormalizer()
			commit := strings.Repeat("a", 40)
			w, err := bundle.NewWriter(filepath.Join(t.TempDir(), "bundle"), bundle.Manifest{CaptureTool: bundle.BuildIdentity{Version: "7.85.0", Commit: commit}, SessionID: "synthetic-output-session", MetricCadences: map[string]time.Duration{"cpu": 15 * time.Second}})
			if err != nil {
				t.Fatal(err)
			}
			profile := schema.Profile{OS: platform, Architecture: "arm64", MemoryBytes: 8 << 30, MetricNames: []string{"system.cpu.user"}, ProcessNames: []string{"Google Chrome"}, SoftwareNames: []string{"Google Chrome"}, Streams: []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Software, schema.AgentInventory, schema.HostInventory}}
			osName := "windows"
			if platform == "macos" {
				osName = "darwin"
			}
			cadences := map[schema.Stream]time.Duration{}
			var requests [][]RecordedRequest
			sequences := map[string]uint64{}
			producerStreams := map[string][]schema.Stream{}
			owner := func(stream schema.Stream) string {
				switch stream {
				case schema.Processes:
					return "process-agent"
				case schema.Connections:
					return "system-probe"
				default:
					return "core-agent"
				}
			}
			save := func(stream schema.Stream, offset time.Duration, value any, send func() error) {
				t.Helper()
				if err := send(); err != nil {
					t.Fatal(err)
				}
				if err := p.Wait(ctx); err != nil {
					t.Fatal(err)
				}
				refs := recorder.Drain()
				if len(refs) == 0 {
					t.Fatal("completed cycle produced no delivery requests")
				}
				role := owner(stream)
				sequences[role]++
				if !slices.Contains(producerStreams[role], stream) {
					producerStreams[role] = append(producerStreams[role], stream)
				}
				ref := bundle.SampleRef{Stream: stream, Offset: offset, ProducerID: "synthetic-" + role, CycleID: sequences[role], Sequence: sequences[role], ChunkCount: 1}
				if err := w.Append(ref, value); err != nil {
					t.Fatal(err)
				}
				requests = append(requests, refs)
				cadences[stream] = 15 * time.Second
			}
			for i := 0; i < 2; i++ {
				offset := time.Duration(i) * 15 * time.Second
				source, err := normalizer.Series(capture.NewSeriesSource([]*metrics.Serie{{Name: "system.cpu.user", Host: "fixture-native-host", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: float64(i * 15), Value: 5}}}}))
				if err != nil {
					t.Fatal(err)
				}
				metricSample, err := telemetry.NewMetricSample(source.(*capture.SeriesSource).Series)
				if err != nil {
					t.Fatal(err)
				}
				save(schema.Metrics, offset, metricSample, func() error { return p.Serializer.SendIterableSeries(source) })
				proc := normalizer.Process(&model.CollectorProc{HostName: "fixture-native-host", GroupSize: 1, Info: &model.SystemInfo{TotalMemory: 8 << 30, Os: &model.OSInfo{Name: osName}}, Processes: []*model.Process{{Pid: 42, Command: &model.Command{Comm: "Google Chrome", Args: []string{"fixture-native-host"}}, Cpu: &model.CPUStat{TotalPct: 3}}}})
				save(schema.Processes, offset, proc, func() error { return p.Process(ctx, time.Unix(int64(i*15), 0), proc) })
				if platform == "windows" {
					conn := normalizer.Connections(&model.CollectorConnections{HostName: "fixture-native-host", GroupSize: 1, Connections: []*model.Connection{{Pid: 42, Laddr: &model.Addr{Ip: "192.0.2.10"}, Raddr: &model.Addr{Ip: "203.0.113.80", Port: 443}, Rtt: 30000}}})
					if i == 0 {
						profile.ConnectionSelectors = []string{telemetry.ConnectionSelector(conn.Connections[0])}
					}
					save(schema.Connections, offset, conn, func() error { return p.Connections(ctx, time.Unix(int64(i*15), 0), conn) })
				}
			}
			host, err := normalizer.HostMetadata(&capture.HostMetadata{Hostname: "fixture-native-host", AgentVersion: "7.85.0", OS: osName, UUID: "fixture-native-host"})
			if err != nil {
				t.Fatal(err)
			}
			save(schema.HostMetadata, 0, host, func() error { return p.Serializer.SendHostMetadata(host) })
			capturedHost := host.(*capture.HostMetadata)
			agentInventory := &telemetrycapture.Inventory{Hostname: capturedHost.Hostname, UUID: capturedHost.UUID, Agent: &telemetrycapture.AgentInventoryMetadata{AgentVersion: capturedHost.AgentVersion, Flavor: "agent", InfrastructureMode: "end_user_device", AgentStartupTimeMS: -3000}}
			save(schema.AgentInventory, 0, agentInventory, func() error { return p.Serializer.SendMetadata(agentInventory) })
			hostInventory := &telemetrycapture.Inventory{Hostname: capturedHost.Hostname, UUID: capturedHost.UUID, Host: &telemetrycapture.HostInventoryMetadata{AgentVersion: capturedHost.AgentVersion, OS: osName, CPUCores: 4, CPULogicalProcessors: 8, MemoryTotalKb: (8 << 30) / 1024}}
			save(schema.HostInventory, 0, hostInventory, func() error { return p.Serializer.SendMetadata(hostInventory) })
			snapshot := &softwareimpl.Payload{Hostname: "fixture-native-host", Metadata: softwareimpl.HostSoftware{Software: normalizer.Software([]software.Entry{{DisplayName: "Google Chrome", Version: "125.0.1", UserSID: "fixture-native-host", ProductCode: "fixture-native-host", InstallPaths: []string{"fixture-native-host"}}})}}
			save(schema.Software, 0, snapshot, func() error {
				body, err := snapshot.MarshalJSON()
				if err != nil {
					return err
				}
				return p.Event(ctx, eventplatform.EventTypeSoftwareInventory, body, time.Unix(0, 0))
			})
			if platform == "windows" {
				profile.Streams = append(profile.Streams, schema.Connections)
			}
			var producers []bundle.Producer
			for _, role := range []string{"core-agent", "process-agent", "system-probe"} {
				if sequences[role] == 0 {
					continue
				}
				producerVersion := "7.85.0-fixture"
				if role == "core-agent" {
					producerVersion = host.(*capture.HostMetadata).AgentVersion
				}
				producers = append(producers, bundle.Producer{Role: role, InstanceID: "synthetic-" + role, Version: producerVersion, Commit: strings.Repeat("b", 40), ProtocolVersion: telemetrycapture.ProtocolVersion, Streams: producerStreams[role], StopOffset: time.Minute, FinalSequence: sequences[role], AcknowledgedSequence: sequences[role], Stopped: true})
			}
			if err := w.SetProducers(producers); err != nil {
				t.Fatal(err)
			}
			loaded, err := w.Complete(time.Minute, profile, cadences)
			if err != nil {
				t.Fatal(err)
			}
			for name, data := range loaded.Files {
				if !strings.Contains(string(data), "fixture-native-host") || strings.Contains(string(data), "recording-only-no-credential") {
					t.Fatalf("native telemetry lost or credential persisted in %s", name)
				}
			}
			// Inspect requests recorded during delivery independently of bundle
			// persistence. Authentication credentials must not enter bodies or recorded headers.
			for i, sample := range loaded.Manifest.Samples {
				for _, reference := range requests[i] {
					name := sample.File
					headers, err := json.Marshal(reference.Headers)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(headers), "recording-only-no-credential") {
						t.Fatalf("credential retained in request headers %s", name)
					}
					var decoded []byte
					switch sample.Stream {
					case schema.Processes, schema.Connections:
						payload, decodeErr := model.DecodeMessage(reference.Body)
						if decodeErr != nil {
							t.Fatalf("decode Agent process message %s: %v", name, decodeErr)
						}
						decoded, err = json.Marshal(payload.Body)
					case schema.Metrics, schema.HostMetadata, schema.AgentInventory, schema.HostInventory:
						if reference.Headers.Get("Content-Encoding") != p.Serializer.Strategy.ContentEncoding() {
							t.Fatalf("unexpected serializer encoding in %s", name)
						}
						decoded, err = p.Serializer.Strategy.Decompress(reference.Body)
					case schema.Software:
						encoding := reference.Headers.Get("Content-Encoding")
						if encoding == "" || encoding == "identity" {
							decoded = reference.Body
						} else {
							kind, supported := map[string]string{"deflate": "zlib", "gzip": "gzip", "zstd": "zstd"}[encoding]
							if !supported {
								t.Fatalf("unexpected event-platform encoding %q in %s", encoding, name)
							}
							compressor := logscompression.NewComponent().NewCompressor(kind, 1)
							if compressor.ContentEncoding() != encoding {
								t.Fatalf("required decompressor %q is unavailable", encoding)
							}
							decoded, err = compressor.Decompress(reference.Body)
						}
					default:
						t.Fatalf("unverified wire stream %s", sample.Stream)
					}
					if err != nil {
						t.Fatalf("decode Agent wire content %s: %v", name, err)
					}
					if sample.Stream == schema.AgentInventory || sample.Stream == schema.HostInventory {
						typed, err := loaded.Decode(sample)
						if err != nil {
							t.Fatal(err)
						}
						var inventory telemetrycapture.Inventory
						if err := json.Unmarshal(decoded, &inventory); err != nil || !reflect.DeepEqual(&inventory, typed.Inventory) {
							t.Fatalf("inventory wire differs from its typed sample: %v", err)
						}
					}
					if strings.Contains(string(decoded), "recording-only-no-credential") {
						t.Fatalf("credential submitted in decoded request body %s", name)
					}
				}
			}
			data, err := json.Marshal(loaded.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "recording-only-no-credential") {
				t.Fatal("credential persisted in manifest")
			}
		})
	}
}
