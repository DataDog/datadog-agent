// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func fixture(t *testing.T, platform string) (string, *Loaded) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	commit := strings.Repeat("a", 40)
	w, err := NewWriter(dir, Manifest{CaptureTool: BuildIdentity{Version: "7.85.0", Commit: commit}, SessionID: "fixture-capture-session", MetricCadences: map[string]time.Duration{"cpu": 15 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	profile := schema.Profile{OS: platform, Architecture: "arm64", MemoryBytes: 8 << 30, Streams: []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Software, schema.AgentInventory, schema.HostInventory}, MetricNames: []string{"system.cpu.user"}, ProcessNames: []string{"Google Chrome"}, SoftwareNames: []string{"OS"}}
	if platform == "windows" {
		profile.Streams = append(profile.Streams, schema.Connections)
		profile.ConnectionSelectors = []string{telemetry.ConnectionSelector(fixtureConnection())}
	}
	cadences := map[schema.Stream]time.Duration{}
	producers := map[string]*Producer{}
	for _, stream := range profile.Streams {
		role := "core-agent"
		if stream == schema.Processes {
			role = "process-agent"
		} else if stream == schema.Connections {
			role = "system-probe"
		}
		producer := producers[role]
		if producer == nil {
			producer = &Producer{Role: role, InstanceID: "fixture-" + role, Version: "7.85.0", Commit: strings.Repeat("b", 40), ProtocolVersion: telemetrycapture.ProtocolVersion, StopOffset: time.Minute, Stopped: true}
			producers[role] = producer
		}
		producer.Streams = append(producer.Streams, stream)
		cadences[stream] = 15 * time.Second
		cycles := 1
		if stream == schema.Metrics || stream == schema.Processes || stream == schema.Connections {
			cycles = 2
		}
		for i := 0; i < cycles; i++ {
			producer.FinalSequence++
			producer.AcknowledgedSequence++
			ref := SampleRef{Stream: stream, Offset: time.Duration(i) * 15 * time.Second, ProducerID: producer.InstanceID, CycleID: producer.FinalSequence, Sequence: producer.FinalSequence, ChunkCount: 1}
			err := w.Append(ref, fixtureSample(t, stream, platform))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var inventory []Producer
	for _, role := range []string{"core-agent", "process-agent", "system-probe"} {
		if producer := producers[role]; producer != nil {
			inventory = append(inventory, *producer)
		}
	}
	if err := w.SetProducers(inventory); err != nil {
		t.Fatal(err)
	}
	loaded, err := w.Complete(time.Minute, profile, cadences)
	if err != nil {
		t.Fatal(err)
	}
	return dir, loaded
}

func fixtureConnection() *model.Connection {
	return &model.Connection{Pid: 100, Laddr: &model.Addr{Ip: "10.0.0.1", Port: 12345}, Raddr: &model.Addr{Ip: "10.0.0.2", Port: 443}}
}

func fixtureSample(t *testing.T, stream schema.Stream, platform string) any {
	t.Helper()
	osname := "darwin"
	if platform == "windows" {
		osname = "windows"
	}
	switch stream {
	case schema.Metrics:
		value, err := telemetry.NewMetricSample([]*metrics.Serie{{Name: "system.cpu.user", Host: "capture-host", Source: metrics.MetricSourceCPU, MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: -0.125, Value: 5}}}})
		if err != nil {
			t.Fatal(err)
		}
		return value
	case schema.HostMetadata:
		if platform == "windows" {
			osname = "win32"
		}
		return &telemetry.HostMetadata{Hostname: "capture-host", UUID: "capture-uuid", AgentVersion: "7.85.0", OS: osname, SystemStats: map[string]json.RawMessage{"cpuCores": json.RawMessage(`8`)}}
	case schema.AgentInventory:
		return &telemetrycapture.Inventory{Hostname: "capture-host", UUID: "capture-uuid", Agent: &telemetrycapture.AgentInventoryMetadata{AgentVersion: "7.85.0", Flavor: "agent", InfrastructureMode: "end_user_device", AgentStartupTimeMS: -3000}}
	case schema.HostInventory:
		return &telemetrycapture.Inventory{Hostname: "capture-host", UUID: "capture-uuid", Host: &telemetrycapture.HostInventoryMetadata{AgentVersion: "7.85.0", OS: osname, CPUCores: 4, CPULogicalProcessors: 8, MemoryTotalKb: (8 << 30) / 1024}}
	case schema.Processes:
		return &model.CollectorProc{HostName: "capture-host", GroupSize: 1, Info: &model.SystemInfo{TotalMemory: 8 << 30, Os: &model.OSInfo{Name: osname}}, Processes: []*model.Process{{Pid: 100, Command: &model.Command{Comm: "Google Chrome"}, CreateTime: -3000}}}
	case schema.Software:
		return &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "OS", Source: "os"}}}}
	case schema.Connections:
		return &model.CollectorConnections{HostName: "capture-host", GroupSize: 1, Connections: []*model.Connection{fixtureConnection()}}
	default:
		t.Fatalf("unsupported test stream %s", stream)
		return nil
	}
}

func TestBundleRoundTripAndPlatformIndependence(t *testing.T) {
	for _, platform := range []string{"windows", "macos"} {
		t.Run(platform, func(t *testing.T) {
			dir, written := fixture(t, platform)
			loaded, err := Load(dir, strings.Repeat("a", 40))
			if err != nil {
				t.Fatal(err)
			}
			if written.Digest != loaded.Digest || loaded.Manifest.Profile.OS != platform || loaded.Manifest.Samples[1].Offset != 15*time.Second {
				t.Fatal("bundle identity, platform, or cadence lost")
			}
			series := loaded.Samples[loaded.Manifest.Samples[0].File].Metrics
			if len(series) != 1 || series[0].Source != metrics.MetricSourceCPU || series[0].Points[0].Ts != -0.125 {
				t.Fatal("typed metric source or relative timestamp lost")
			}
		})
	}
}

func TestRejectTamperedOrIncompatibleBundles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"schema", func(m *Manifest) { m.SchemaVersion++ }},
		{"revision", func(m *Manifest) { m.CaptureTool.Commit = strings.Repeat("b", 40) }},
		{"completion", func(m *Manifest) { m.Complete = false }},
		{"missing stream", func(m *Manifest) { m.Profile.Streams = m.Profile.Streams[:1] }},
		{"checksum", func(m *Manifest) {
			for k := range m.Files {
				m.Files[k] = strings.Repeat("0", 64)
				break
			}
		}},
		{"invalid cadence", func(m *Manifest) { m.Cadences[schema.Metrics] = 0 }},
		{"invalid offset", func(m *Manifest) { m.Samples[0].Offset = -1 }},
		{"invented metric", func(m *Manifest) { m.Profile.MetricNames = append(m.Profile.MetricNames, "system.wlan.rssi") }},
		{"omitted metric", func(m *Manifest) { m.Profile.MetricNames = nil }},
		{"invented process", func(m *Manifest) { m.Profile.ProcessNames = []string{"SentinelAgent.exe"} }},
		{"omitted software", func(m *Manifest) { m.Profile.SoftwareNames = nil }},
		{"invented connection", func(m *Manifest) { m.Profile.ConnectionSelectors = []string{"100:10.0.0.1:12345>10.0.0.2:443"} }},
		{"duplicate inventory", func(m *Manifest) { m.Profile.SoftwareNames = []string{"OS", "OS"} }},
		{"memory mismatch", func(m *Manifest) { m.Profile.MemoryBytes++ }},
		{"platform mismatch", func(m *Manifest) { m.Profile.OS = "windows" }},
		{"unknown cadence", func(m *Manifest) { m.Cadences[schema.Stream("extra")] = time.Second }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, loaded := fixture(t, "macos")
			m := loaded.Manifest
			tc.mutate(&m)
			writeManifest(t, dir, m)
			if _, err := Load(dir, strings.Repeat("a", 40)); err == nil {
				t.Fatal("accepted invalid capture")
			}
		})
	}
	dir, _ := fixture(t, "macos")
	if err := os.Remove(filepath.Join(dir, "COMPLETE")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, strings.Repeat("a", 40)); err == nil {
		t.Fatal("accepted interrupted capture")
	}
}

func writeManifest(t *testing.T, directory string, manifest Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "COMPLETE"), []byte(schema.Digest(data)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRejectInvalidTypedSamplesWithValidChecksums(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stream   schema.Stream
		platform string
		value    any
	}{
		{"malformed JSON", schema.Metrics, "macos", []byte(`{`)},
		{"empty object", schema.Processes, "macos", map[string]any{}},
		{"null", schema.HostMetadata, "macos", nil},
		{"wrong stream", schema.Software, "macos", &model.CollectorProc{HostName: "capture-host"}},
		{"empty metrics", schema.Metrics, "macos", telemetry.MetricSample{}},
		{"legacy lossy metric JSON", schema.Metrics, "macos", []*metrics.Serie{{Name: "system.cpu.user", Host: "capture-host"}}},
		{"wrong host platform", schema.HostMetadata, "macos", &telemetry.HostMetadata{Hostname: "capture-host", AgentVersion: "7.85.0", OS: "windows"}},
		{"empty processes", schema.Processes, "macos", &model.CollectorProc{HostName: "capture-host", Info: &model.SystemInfo{TotalMemory: 8 << 30, Os: &model.OSInfo{Name: "darwin"}}}},
		{"unnamed process", schema.Processes, "macos", &model.CollectorProc{HostName: "capture-host", Info: &model.SystemInfo{TotalMemory: 8 << 30, Os: &model.OSInfo{Name: "darwin"}}, Processes: []*model.Process{{Pid: 100}}}},
		{"empty software", schema.Software, "macos", &softwareimpl.Payload{Hostname: "capture-host"}},
		{"empty connections", schema.Connections, "windows", &model.CollectorConnections{HostName: "capture-host"}},
		{"nil connection", schema.Connections, "windows", &model.CollectorConnections{HostName: "capture-host", Connections: []*model.Connection{nil}}},
		{"missing address", schema.Connections, "windows", &model.CollectorConnections{HostName: "capture-host", Connections: []*model.Connection{{Pid: 100}}}},
		{"inconsistent host", schema.Software, "macos", &softwareimpl.Payload{Hostname: "another-host", Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "OS", Source: "os"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, loaded := fixture(t, tc.platform)
			data, ok := tc.value.([]byte)
			if !ok {
				var err error
				data, err = json.Marshal(tc.value)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, ref := range loaded.Manifest.Samples {
				if ref.Stream != tc.stream {
					continue
				}
				if err := os.WriteFile(filepath.Join(dir, ref.File), data, 0600); err != nil {
					t.Fatal(err)
				}
				loaded.Manifest.Files[ref.File] = schema.Digest(data)
				break
			}
			writeManifest(t, dir, loaded.Manifest)
			if _, err := Load(dir, strings.Repeat("a", 40)); err == nil {
				t.Fatal("accepted invalid typed sample with matching checksums")
			}
		})
	}
}

func TestNativeSoftwareSourceIsNotRestrictedToKnownCollectors(t *testing.T) {
	dir, loaded := fixture(t, "macos")
	for _, ref := range loaded.Manifest.Samples {
		if ref.Stream != schema.Software {
			continue
		}
		value := loaded.Samples[ref.File].Software
		value.Metadata.Software[0].Source = "enterprise_catalog"
		writeBundleFile(t, dir, loaded, ref.File, value)
	}
	writeManifest(t, dir, loaded.Manifest)
	if _, err := Load(dir, strings.Repeat("a", 40)); err != nil {
		t.Fatalf("native software source was rejected: %v", err)
	}
}
