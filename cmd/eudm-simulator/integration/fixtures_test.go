// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package integration

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Synthetic inputs, serialized by real Agent delivery packages. This fixture
// commit deliberately cannot be used by a revision-stamped staging binary.
const fixtureCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestGenerateCaptureFixtures(t *testing.T) {
	if os.Getenv("EUDM_GENERATE_FIXTURES") != "1" {
		t.Skip("explicit fixture generation only")
	}
	outputDirectory := os.Getenv("EUDM_FIXTURE_OUTPUT")
	if outputDirectory == "" && os.Getenv("TEST_UNDECLARED_OUTPUTS_DIR") != "" {
		outputDirectory = filepath.Join(os.Getenv("TEST_UNDECLARED_OUTPUTS_DIR"), "eudm-bundles")
	}
	if outputDirectory == "" || !filepath.IsAbs(outputDirectory) {
		t.Fatal("fixture generation requires an absolute EUDM_FIXTURE_OUTPUT or Bazel undeclared-output directory")
	}
	for _, platform := range []string{"macos", "windows"} {
		t.Run(platform, func(t *testing.T) { generateFixture(t, platform, outputDirectory) })
	}
}
func generateFixture(t *testing.T, platform, outputDirectory string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	recorder := output.NewRecorder()
	p, err := output.New(ctx, nil, "synthetic-fixture-no-credential", recorder, output.Options{MetricProtocol: "v2", MetadataProtocol: "metadata-v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	directory := filepath.Join(outputDirectory, platform)
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		t.Fatal(err)
	}
	metricCadences := map[string]time.Duration{"cpu": 15 * time.Second, "memory": 15 * time.Second, "wlan": 15 * time.Second, "network": 15 * time.Second, "battery": 5 * time.Minute}
	const captureDuration = 5*time.Minute + time.Second
	w, err := bundle.NewWriter(directory, bundle.Manifest{CaptureTool: bundle.BuildIdentity{Version: "7.85.0-fixture", Commit: fixtureCommit}, SessionID: "synthetic-fixture-session", MetricCadences: metricCadences})
	if err != nil {
		t.Fatal(err)
	}
	osname, hostOS, arch, chrome := "darwin", "darwin", "arm64", "Google Chrome"
	if platform == "windows" {
		osname, hostOS, arch, chrome = "windows", "win32", "amd64", "chrome.exe"
	}
	background := "process-" + strings.Repeat("1", 32)
	profile := schema.Profile{OS: platform, Architecture: arch, MemoryBytes: 16 << 30, Streams: []schema.Stream{schema.Metrics, schema.HostMetadata, schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo, schema.Processes, schema.Connections, schema.Software}, ProcessNames: []string{chrome, background}, SoftwareNames: []string{"Google Chrome", "OS"}}
	if platform == "windows" {
		profile.ProcessNames = append(profile.ProcessNames, "SentinelAgent.exe")
		profile.SoftwareNames = append(profile.SoftwareNames, "SentinelOne")
	}
	cadences := map[schema.Stream]time.Duration{schema.Metrics: 15 * time.Second, schema.Processes: 10 * time.Second, schema.Connections: 10 * time.Second, schema.HostMetadata: 5 * time.Minute, schema.AgentInventory: 10 * time.Minute, schema.HostInventory: 10 * time.Minute, schema.HostSystemInfo: time.Hour, schema.Software: 10 * time.Minute}
	sequences := map[string]uint64{}
	producerStreams := map[string][]schema.Stream{}
	owner := func(stream schema.Stream) string {
		switch stream {
		case schema.Processes:
			return "process-agent"
		case schema.Connections:
			if platform == "windows" {
				return "system-probe"
			}
			return "process-agent"
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
			t.Fatal("synthetic cycle produced no wire evidence")
		}
		role := owner(stream)
		sequences[role]++
		if !slices.Contains(producerStreams[role], stream) {
			producerStreams[role] = append(producerStreams[role], stream)
		}
		ref := bundle.SampleRef{Stream: stream, Offset: offset, ProducerID: "synthetic-" + role, CycleID: sequences[role], Sequence: sequences[role], ChunkCount: 1}
		if stream == schema.Metrics || stream == schema.HostMetadata || stream == schema.AgentInventory || stream == schema.HostInventory || stream == schema.HostSystemInfo {
			var ordinals []uint64
			if metric, ok := value.(*telemetry.MetricSample); ok {
				for i := range metric.Series {
					ordinals = append(ordinals, uint64(i+1))
				}
			}
			for i, wire := range refs {
				protocol := map[string]string{"/api/v1/series": "v1", "/api/v2/series": "v2", "/api/intake/metrics/v3/series": "v3", "/api/intake/metrics/v3beta/series": "v3beta", "/intake/": "metadata-v1", "/api/v2/host_metadata": "metadata-v2", "/api/v1/metadata": "inventory-v1"}[wire.Path]
				if protocol == "" {
					t.Fatal("unexpected synthetic wire endpoint")
				}
				ref.Routes = append(ref.Routes, bundle.RoutingEvidence{PayloadID: uint64(i + 1), Ordinals: ordinals, Endpoint: wire.Path, Protocol: protocol, Destination: "primary", EnqueueOffset: offset})
			}
		}
		if err := w.Append(ref, value, refs); err != nil {
			t.Fatal(err)
		}
	}
	values := map[string]float64{
		"system.cpu.user": 5, "system.cpu.system": 2, "system.cpu.idle": 93, "system.cpu.num_cores": 4,
		"system.mem.total": 16384, "system.mem.used": 4096, "system.mem.free": 12288, "system.mem.usable": 12288, "system.mem.pct_usable": 0.75,
		"system.wlan.rssi": -55, "system.wlan.noise": -95, "system.wlan.txrate": 600, "system.wlan.rxrate": 600, "system.wlan.status": 1,
		"system.net.bytes_sent": 1024, "system.net.bytes_rcvd": 4096,
		"system.battery.maximum_capacity_pct": 96, "system.battery.current_charge_pct": 75, "system.battery.cycle_count": 120, "system.battery.charge_rate": -4,
	}
	for name := range values {
		if metricCadences[tc.MetricFamily(name)] <= 0 {
			t.Fatalf("synthetic metric %s has no supported family cadence", name)
		}
		profile.MetricNames = append(profile.MetricNames, name)
	}
	sort.Strings(profile.MetricNames)
	// A native five-minute battery check can share a serializer flush with
	// faster families. Keep every fast cycle and the two distinct battery
	// observations so replay cannot infer battery cadence from CPU flushes.
	for offset := time.Duration(0); offset < captureDuration; offset += 15 * time.Second {
		var series []*metrics.Serie
		for _, name := range profile.MetricNames {
			family := tc.MetricFamily(name)
			cadence := metricCadences[family]
			if offset%cadence != 0 {
				continue
			}
			source := metrics.MetricSourceCPU
			metricType := metrics.APIGaugeType
			tags := []string{"infra_mode:end_user_device"}
			switch family {
			case "memory":
				source = metrics.MetricSourceMemory
			case "wlan":
				source = metrics.MetricSourceWlan
				tags = append(tags, "bssid:02:00:00:00:00:01", "mac_address:02:00:00:00:00:02", "ssid:ssid-fixture", "interface:interface-"+strings.Repeat("2", 32))
			case "network":
				source = metrics.MetricSourceNetwork
				metricType = metrics.APIRateType
				tags = append(tags, "device:interface-"+strings.Repeat("2", 32))
			case "battery":
				source = metrics.MetricSourceBattery
			}
			series = append(series, &metrics.Serie{Name: name, Host: "capture-host", MType: metricType, Source: source, Interval: int64(cadence / time.Second), Tags: tagset.CompositeTagsFromSlice(tags), Points: []metrics.Point{{Ts: offset.Seconds(), Value: values[name]}}})
		}
		envelope, err := telemetry.NewMetricSample(series)
		if err != nil {
			t.Fatal(err)
		}
		save(schema.Metrics, offset, envelope, func() error { return p.Serializer.SendIterableSeries(capture.NewSeriesSource(series)) })
	}
	process := func(pid int32, name string, cpu float32, rss uint64) *model.Process {
		exe := "/capture/bin/" + name
		if platform == "windows" {
			exe = `C:\capture\bin\` + name
		}
		return &model.Process{Pid: pid, CreateTime: -60000, Command: &model.Command{Comm: name, Exe: exe, Args: []string{exe}}, User: &model.ProcessUser{Name: "user-" + strings.Repeat("3", 32)}, Cpu: &model.CPUStat{TotalPct: cpu, UserPct: cpu * .75, SystemPct: cpu * .25, NumThreads: 2}, Memory: &model.MemoryStat{Rss: rss, Vms: 2 * rss}}
	}
	for cycle := 0; cycle < 2; cycle++ {
		offset := time.Duration(cycle) * 10 * time.Second
		proc := &model.CollectorProc{HostName: "capture-host", NetworkId: "network-" + strings.Repeat("4", 32), GroupId: int32(cycle + 1), GroupSize: 1, Info: &model.SystemInfo{Uuid: "00000000-0000-4000-8000-000000000001", Os: &model.OSInfo{Name: osname, Version: "15.6"}, TotalMemory: 16 << 30, Cpus: []*model.CPUInfo{{Cores: 4}}}, Processes: []*model.Process{process(100, chrome, 8, 300<<20), process(300, background, 3, 100<<20)}}
		if platform == "windows" {
			proc.Processes = append(proc.Processes, process(200, "SentinelAgent.exe", 4, 200<<20))
		}
		save(schema.Processes, offset, proc, func() error { return p.Process(ctx, time.Unix(int64(offset.Seconds()), 0), proc) })
		conn := &model.CollectorConnections{HostName: "capture-host", NetworkId: proc.NetworkId, GroupId: int32(cycle + 1), GroupSize: 1, Connections: []*model.Connection{{Pid: 300, Laddr: &model.Addr{Ip: "10.0.0.1", Port: 50000}, Raddr: &model.Addr{Ip: "10.0.0.2", Port: 443}, Type: model.ConnectionType_tcp, Rtt: 20000, RttVar: 2000, LastBytesSent: 1000, LastBytesReceived: 2000}}}
		// Backend routing distinguishes an EUDM-only macOS sender from the
		// synthetic Windows sender that also has NPM enabled. A nil config
		// defaults to NPM in intake, so preserve the explicit false flag.
		conn.AgentConfiguration = &model.AgentConfiguration{EudmEnabled: true, NpmEnabled: platform == "windows"}
		if cycle == 0 {
			profile.ConnectionSelectors = []string{telemetry.ConnectionSelector(conn.Connections[0])}
		}
		save(schema.Connections, offset, conn, func() error { return p.Connections(ctx, time.Unix(int64(offset.Seconds()), 0), conn) })
	}
	host := &telemetry.HostMetadata{Hostname: "capture-host", UUID: "00000000-0000-4000-8000-000000000001", AgentVersion: "7.85.0-fixture", AgentFlavor: "agent", OS: hostOS, HostTags: map[string][]string{"system": {"infra_mode:end_user_device"}}}
	save(schema.HostMetadata, 0, host, func() error { return p.Serializer.SendHostMetadata(host) })
	agentInventory := &tc.Inventory{Hostname: host.Hostname, UUID: host.UUID, Timestamp: int64(250 * time.Millisecond), Agent: &tc.AgentInventoryMetadata{
		AgentVersion: host.AgentVersion, PackageVersion: "7.85.0-fixture", Flavor: "agent", InfrastructureMode: "end_user_device", AgentStartupTimeMS: -60000,
		FeatureProcessEnabled: true, FeatureNetworksEnabled: true,
	}}
	save(schema.AgentInventory, 0, agentInventory, func() error { return p.Serializer.SendMetadata(agentInventory) })
	inventoryOS := osname
	if platform == "windows" {
		inventoryOS = "Microsoft Windows 11 Pro"
	}
	hostInventory := &tc.Inventory{Hostname: host.Hostname, UUID: host.UUID, Timestamp: int64(500 * time.Millisecond), Host: &tc.HostInventoryMetadata{
		CPUCores: 4, CPULogicalProcessors: 4, CPUArchitecture: arch, CPUVendor: "fixture-vendor", CPUModel: "fixture-model",
		MemoryTotalKb: 16 << 20, KernelName: osname, OS: inventoryOS, OSVersion: "15.6", AgentVersion: host.AgentVersion,
		IPAddress: "10.0.0.1", MacAddress: "02:00:00:00:00:02",
	}}
	save(schema.HostInventory, 0, hostInventory, func() error { return p.Serializer.SendMetadata(hostInventory) })
	systemInfo := &tc.Inventory{Hostname: host.Hostname, UUID: host.UUID, Timestamp: int64(5250 * time.Millisecond), SystemInfo: &tc.HostSystemInfoMetadata{
		Manufacturer: "Apple Inc.", ModelNumber: "Mac16,5", SerialNumber: "serial_number-" + strings.Repeat("5", 32),
		ModelName: "device_model-" + strings.Repeat("6", 32), ChassisType: "Laptop", Identifier: "Mac16,5",
	}}
	if platform == "windows" {
		systemInfo.SystemInfo.Manufacturer = "Lenovo"
		systemInfo.SystemInfo.ModelNumber = "device_model-" + strings.Repeat("7", 32)
		systemInfo.SystemInfo.Identifier = "device_model-" + strings.Repeat("8", 32)
	}
	save(schema.HostSystemInfo, 5*time.Second, systemInfo, func() error { return p.Serializer.SendMetadata(systemInfo) })
	kind := "app"
	if platform == "windows" {
		kind = "desktop"
	}
	sw := &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "Google Chrome", Version: "137.0.7151.69", Source: kind, Status: "installed"}, {DisplayName: "OS", Version: "15.6", Source: "os", Status: "installed"}}}}
	if platform == "windows" {
		sw.Metadata.Software = append(sw.Metadata.Software, software.Entry{DisplayName: "SentinelOne", Version: "23.4.2", Source: "desktop", Status: "installed"})
	}
	save(schema.Software, 0, sw, func() error {
		body, err := sw.MarshalJSON()
		if err != nil {
			return err
		}
		return p.Event(ctx, eventplatform.EventTypeSoftwareInventory, body, time.Unix(0, 0))
	})
	sort.Strings(profile.ProcessNames)
	sort.Strings(profile.SoftwareNames)
	var producers []bundle.Producer
	for _, role := range []string{"core-agent", "process-agent", "system-probe"} {
		if sequences[role] == 0 {
			continue
		}
		producers = append(producers, bundle.Producer{Role: role, InstanceID: "synthetic-" + role, Version: "7.85.0-fixture", Commit: strings.Repeat("b", 40), ProtocolVersion: 1, Streams: producerStreams[role], StopOffset: captureDuration, FinalSequence: sequences[role], AcknowledgedSequence: sequences[role], Stopped: true})
	}
	if err := w.SetProducers(producers); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Complete(captureDuration, profile, cadences); err != nil {
		t.Fatal(err)
	}
}
