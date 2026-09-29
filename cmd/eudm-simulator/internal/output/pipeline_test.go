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
)

func TestPortableSanitizedBundleRoundTripThroughAgentDelivery(t *testing.T) {
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
			sanitizer, err := capture.NewSanitizer()
			if err != nil {
				t.Fatal(err)
			}
			commit := strings.Repeat("a", 40)
			w, err := bundle.NewWriter(filepath.Join(t.TempDir(), "bundle"), bundle.Manifest{AgentCommit: commit, AgentVersion: "7.85.0"})
			if err != nil {
				t.Fatal(err)
			}
			profile := schema.Profile{OS: platform, Architecture: "arm64", MemoryBytes: 8 << 30, MetricNames: []string{"system.cpu.user"}, ProcessNames: []string{"Google Chrome"}, SoftwareNames: []string{"Google Chrome"}, Streams: []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Software}}
			osName := "windows"
			if platform == "macos" {
				osName = "darwin"
			}
			cadences := map[schema.Stream]time.Duration{}
			previous := 0
			save := func(stream schema.Stream, offset time.Duration, value any, send func() error) {
				t.Helper()
				if err := send(); err != nil {
					t.Fatal(err)
				}
				if err := p.Wait(ctx); err != nil {
					t.Fatal(err)
				}
				refs, err := recorder.Wait(ctx, previous+1)
				if err != nil {
					t.Fatal(err)
				}
				if err := w.Append(stream, offset, value, refs[previous:]); err != nil {
					t.Fatal(err)
				}
				previous = len(refs)
				cadences[stream] = 15 * time.Second
			}
			for i := 0; i < 2; i++ {
				offset := time.Duration(i) * 15 * time.Second
				source, err := sanitizer.Series(capture.NewSeriesSource([]*metrics.Serie{{Name: "system.cpu.user", Host: "UNIQUE-CAPTURE-SECRET", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: float64(i * 15), Value: 5}}}}))
				if err != nil {
					t.Fatal(err)
				}
				metricSample, err := telemetry.NewMetricSample(source.(*capture.SeriesSource).Series)
				if err != nil {
					t.Fatal(err)
				}
				save(schema.Metrics, offset, metricSample, func() error { return p.Serializer.SendIterableSeries(source) })
				proc := sanitizer.Process(&model.CollectorProc{HostName: "UNIQUE-CAPTURE-SECRET", GroupSize: 1, Info: &model.SystemInfo{TotalMemory: 8 << 30, Os: &model.OSInfo{Name: osName}}, Processes: []*model.Process{{Pid: 42, Command: &model.Command{Comm: "Google Chrome", Args: []string{"UNIQUE-CAPTURE-SECRET"}}, Cpu: &model.CPUStat{TotalPct: 3}}}})
				save(schema.Processes, offset, proc, func() error { return p.Process(ctx, time.Unix(int64(i*15), 0), proc) })
				if platform == "windows" {
					conn := sanitizer.Connections(&model.CollectorConnections{HostName: "UNIQUE-CAPTURE-SECRET", GroupSize: 1, Connections: []*model.Connection{{Pid: 42, Laddr: &model.Addr{Ip: "UNIQUE-CAPTURE-SECRET"}, Raddr: &model.Addr{Ip: "UNIQUE-CAPTURE-SECRET", Port: 443}, Rtt: 30000}}})
					if i == 0 {
						profile.ConnectionSelectors = []string{telemetry.ConnectionSelector(conn.Connections[0])}
					}
					save(schema.Connections, offset, conn, func() error { return p.Connections(ctx, time.Unix(int64(i*15), 0), conn) })
				}
			}
			host, err := sanitizer.HostMetadata(&capture.HostMetadata{Hostname: "UNIQUE-CAPTURE-SECRET", AgentVersion: "7.85.0", OS: osName, UUID: "UNIQUE-CAPTURE-SECRET"})
			if err != nil {
				t.Fatal(err)
			}
			save(schema.HostMetadata, 0, host, func() error { return p.Serializer.SendHostMetadata(host) })
			snapshot := &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: sanitizer.Software([]software.Entry{{DisplayName: "Google Chrome", Version: "125.0.1", UserSID: "UNIQUE-CAPTURE-SECRET", ProductCode: "UNIQUE-CAPTURE-SECRET", InstallPaths: []string{"UNIQUE-CAPTURE-SECRET"}}})}}
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
			loaded, err := w.Complete(time.Minute, profile, cadences)
			if err != nil {
				t.Fatal(err)
			}
			for name, data := range loaded.Files {
				if strings.Contains(string(data), "UNIQUE-CAPTURE-SECRET") {
					t.Fatalf("raw identity persisted in %s", name)
				}
			}
			// Wire files encode bytes as base64, and Agent bodies may themselves
			// be compressed. Inspect the decoded wire content as well as the JSON
			// files so neither layer can conceal a raw capture identity.
			for _, sample := range loaded.Manifest.Samples {
				for _, name := range sample.WireFiles {
					var reference bundle.WireReference
					if err := json.Unmarshal(loaded.Files[name], &reference); err != nil {
						t.Fatal(err)
					}
					headers, err := json.Marshal(reference.Headers)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(headers), "UNIQUE-CAPTURE-SECRET") {
						t.Fatalf("raw identity persisted in wire headers %s", name)
					}
					var decoded []byte
					switch sample.Stream {
					case schema.Processes, schema.Connections:
						payload, decodeErr := model.DecodeMessage(reference.Body)
						if decodeErr != nil {
							t.Fatalf("decode Agent process message %s: %v", name, decodeErr)
						}
						decoded, err = json.Marshal(payload.Body)
					case schema.Metrics, schema.HostMetadata:
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
					if strings.Contains(string(decoded), "UNIQUE-CAPTURE-SECRET") {
						t.Fatalf("raw identity persisted in decoded wire body %s", name)
					}
				}
			}
			data, err := json.Marshal(loaded.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "UNIQUE-CAPTURE-SECRET") {
				t.Fatal("raw identity persisted in manifest")
			}
		})
	}
}
