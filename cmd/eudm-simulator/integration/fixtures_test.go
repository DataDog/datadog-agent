// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package integration

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Synthetic typed inputs, replayed through real Agent delivery in tests. This fixture
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
	generateFixtureDuration(t, platform, outputDirectory, 5*time.Minute+time.Second)
}

func generateFixtureDuration(t *testing.T, platform, outputDirectory string, captureDuration time.Duration) {
	t.Helper()
	directory := filepath.Join(outputDirectory, platform)
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		t.Fatal(err)
	}
	metricCadences := map[string]time.Duration{"cpu": 15 * time.Second, "memory": 15 * time.Second, "wlan": 15 * time.Second, "network": 15 * time.Second, "battery": 5 * time.Minute}
	w, err := bundle.NewWriter(directory, bundle.Manifest{CaptureTool: bundle.BuildIdentity{Version: "7.85.0-fixture", Commit: fixtureCommit}, SessionID: "synthetic-fixture-session", MetricCadences: metricCadences})
	if err != nil {
		t.Fatal(err)
	}
	osname, hostOS, arch, chrome := "darwin", "darwin", "arm64", "Google Chrome"
	if platform == "windows" {
		osname, hostOS, arch, chrome = "windows", "win32", "amd64", "chrome.exe"
	}
	background := "AcmeSync"
	if platform == "windows" {
		background += ".exe"
	}
	profile := schema.Profile{OS: platform, Architecture: arch, MemoryBytes: 16 << 30, Streams: []schema.Stream{schema.Metrics, schema.HostMetadata, schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo, schema.Processes, schema.Connections, schema.Software}, ProcessNames: []string{chrome, background}, SoftwareNames: []string{"Google Chrome", "OS", "Acme Workspace"}}
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
	save := func(stream schema.Stream, offset time.Duration, value any) {
		t.Helper()
		role := owner(stream)
		sequences[role]++
		if !slices.Contains(producerStreams[role], stream) {
			producerStreams[role] = append(producerStreams[role], stream)
		}
		ref := bundle.SampleRef{Stream: stream, Offset: offset, ProducerID: "synthetic-" + role, CycleID: sequences[role], Sequence: sequences[role], ChunkCount: 1}
		if err := w.Append(ref, value); err != nil {
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
				tags = append(tags, "bssid:02:00:00:00:00:01", "mac_address:02:00:00:00:00:02", "ssid:Example Office", "interface:en0")
			case "network":
				source = metrics.MetricSourceNetwork
				metricType = metrics.APIRateType
				tags = append(tags, "device:en0")
			case "battery":
				source = metrics.MetricSourceBattery
			}
			series = append(series, &metrics.Serie{Name: name, Host: "capture-host", MType: metricType, Source: source, Interval: int64(cadence / time.Second), Tags: tagset.CompositeTagsFromSlice(tags), Points: []metrics.Point{{Ts: offset.Seconds(), Value: values[name]}}})
		}
		envelope, err := telemetry.NewMetricSample(series)
		if err != nil {
			t.Fatal(err)
		}
		save(schema.Metrics, offset, envelope)
	}
	process := func(pid int32, name string, cpu float32, rss uint64) *model.Process {
		exe := "/Applications/" + name + ".app/Contents/MacOS/" + name
		if platform == "windows" {
			exe = `C:\Program Files\Acme\` + name
		}
		return &model.Process{Pid: pid, CreateTime: -60000, Command: &model.Command{Comm: name, Exe: exe, Args: []string{exe, "--profile=work"}}, User: &model.ProcessUser{Name: "fixture-user"}, Cpu: &model.CPUStat{TotalPct: cpu, UserPct: cpu * .75, SystemPct: cpu * .25, NumThreads: 2}, Memory: &model.MemoryStat{Rss: rss, Vms: 2 * rss}, IoStat: &model.IOStat{ReadRate: 5, WriteRate: 9}, Tags: []string{"team:desktop", "interactive"}}
	}
	for cycle, offset := 0, time.Duration(0); offset < captureDuration; cycle, offset = cycle+1, offset+10*time.Second {
		proc := &model.CollectorProc{HostName: "capture-host", NetworkId: "network-" + strings.Repeat("4", 32), GroupId: int32(cycle + 1), GroupSize: 1, Info: &model.SystemInfo{Uuid: "00000000-0000-4000-8000-000000000001", Os: &model.OSInfo{Name: osname, Version: "15.6"}, TotalMemory: 16 << 30, Cpus: []*model.CPUInfo{{Cores: 4}}}, Processes: []*model.Process{process(100, chrome, 8, 300<<20), process(300, background, 3, 100<<20)}}
		proc.Hints = &model.CollectorProc_HintMask{HintMask: 1}
		if platform == "windows" {
			proc.Processes = append(proc.Processes, process(200, "SentinelAgent.exe", 4, 200<<20))
		}
		save(schema.Processes, offset, proc)
		conn := &model.CollectorConnections{HostName: "capture-host", NetworkId: proc.NetworkId, GroupId: int32(cycle + 1), GroupSize: 1, Connections: []*model.Connection{{Pid: 300, Laddr: &model.Addr{Ip: "10.0.0.1", Port: 50000}, Raddr: &model.Addr{Ip: "203.0.113.80", Port: 443}, Type: model.ConnectionType_tcp, Rtt: 20000, RttVar: 2000, LastBytesSent: 1000, LastBytesReceived: 2000}}}
		dns := model.NewV2DNSEncoder()
		domains, offsets, err := dns.EncodeDomainDatabase([]string{"api.acme.example"})
		if err != nil {
			t.Fatal(err)
		}
		lookups, err := dns.EncodeMapped(map[string]*model.DNSDatabaseEntry{"203.0.113.80": {NameOffsets: []int32{0}}}, offsets)
		if err != nil {
			t.Fatal(err)
		}
		conn.EncodedDomainDatabase, conn.EncodedDnsLookups = domains, lookups
		// Backend routing distinguishes an EUDM-only macOS sender from the
		// synthetic Windows sender that also has NPM enabled. A nil config
		// defaults to NPM in intake, so preserve the explicit false flag.
		conn.AgentConfiguration = &model.AgentConfiguration{EudmEnabled: true, NpmEnabled: platform == "windows"}
		if cycle == 0 {
			profile.ConnectionSelectors = []string{telemetry.ConnectionSelector(conn.Connections[0])}
		}
		save(schema.Connections, offset, conn)
	}
	host := &telemetry.HostMetadata{Hostname: "capture-host", UUID: "00000000-0000-4000-8000-000000000001", AgentVersion: "7.85.0-fixture", AgentFlavor: "agent", OS: hostOS, HostTags: map[string][]string{"system": {"infra_mode:end_user_device"}, "gcp": {"team:desktop", "interactive"}}}
	for offset := time.Duration(0); offset < captureDuration; offset += cadences[schema.HostMetadata] {
		save(schema.HostMetadata, offset, host)
	}
	agentInventory := &tc.Inventory{Hostname: host.Hostname, UUID: host.UUID, Timestamp: int64(250 * time.Millisecond), Agent: &tc.AgentInventoryMetadata{
		AgentVersion: host.AgentVersion, PackageVersion: "7.85.0-fixture", Flavor: "agent", InfrastructureMode: "end_user_device", AgentStartupTimeMS: -60000,
		FeatureProcessEnabled: true, FeatureNetworksEnabled: true,
	}}
	for offset := time.Duration(0); offset < captureDuration; offset += cadences[schema.AgentInventory] {
		agentInventory.Timestamp = int64(offset + 250*time.Millisecond)
		save(schema.AgentInventory, offset, agentInventory)
	}
	inventoryOS := osname
	if platform == "windows" {
		inventoryOS = "Microsoft Windows 11 Pro"
	}
	hostInventory := &tc.Inventory{Hostname: host.Hostname, UUID: host.UUID, Timestamp: int64(500 * time.Millisecond), Host: &tc.HostInventoryMetadata{
		CPUCores: 4, CPULogicalProcessors: 4, CPUArchitecture: arch, CPUVendor: "fixture-vendor", CPUModel: "fixture-model",
		MemoryTotalKb: 16 << 20, KernelName: osname, OS: inventoryOS, OSVersion: "15.6", AgentVersion: host.AgentVersion,
		IPAddress: "10.0.0.1", MacAddress: "02:00:00:00:00:02",
	}}
	for offset := time.Duration(0); offset < captureDuration; offset += cadences[schema.HostInventory] {
		hostInventory.Timestamp = int64(offset + 500*time.Millisecond)
		save(schema.HostInventory, offset, hostInventory)
	}
	systemInfo := &tc.Inventory{Hostname: host.Hostname, UUID: host.UUID, Timestamp: int64(5250 * time.Millisecond), SystemInfo: &tc.HostSystemInfoMetadata{
		Manufacturer: "Apple Inc.", ModelNumber: "Mac16,5", SerialNumber: "FIXTURE-SERIAL-001",
		ModelName: "MacBook Pro", ChassisType: "Laptop", Identifier: "Mac16,5",
	}}
	if platform == "windows" {
		systemInfo.SystemInfo.Manufacturer = "Lenovo"
		systemInfo.SystemInfo.ModelNumber = "21HM"
		systemInfo.SystemInfo.ModelName = "ThinkPad X1 Carbon Gen 11"
		systemInfo.SystemInfo.Identifier = "21HMCTO1WW"
	}
	for offset := 5 * time.Second; offset < captureDuration; offset += cadences[schema.HostSystemInfo] {
		systemInfo.Timestamp = int64(offset + 250*time.Millisecond)
		save(schema.HostSystemInfo, offset, systemInfo)
	}
	kind := "app"
	if platform == "windows" {
		kind = "desktop"
	}
	sw := &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "Google Chrome", Version: "137.0.7151.69", Source: kind, Status: "installed"}, {DisplayName: "OS", Version: "15.6", Source: "os", Status: "installed"}}}}
	productCode, installPath := "com.acme.workspace", "/Applications/Acme Workspace.app"
	if platform == "windows" {
		productCode, installPath = "{28DA55C3-E174-49F3-8411-441CFABEF0D2}", `C:\Program Files\Acme\Workspace`
	}
	sw.Metadata.Software = append(sw.Metadata.Software, software.Entry{DisplayName: "Acme Workspace", Publisher: "Acme Software Ltd.", Version: "2025.10-beta+build.7", Source: kind, Status: "installed", ProductCode: productCode, InstallDate: "2025-06-12T10:30:00Z", InstallPaths: []string{installPath}})
	if platform == "windows" {
		sw.Metadata.Software = append(sw.Metadata.Software, software.Entry{DisplayName: "SentinelOne", Version: "23.4.2", Source: "desktop", Status: "installed"})
	}
	for offset := time.Duration(0); offset < captureDuration; offset += cadences[schema.Software] {
		save(schema.Software, offset, sw)
	}
	sort.Strings(profile.ProcessNames)
	sort.Strings(profile.SoftwareNames)
	var producers []bundle.Producer
	for _, role := range []string{"core-agent", "process-agent", "system-probe"} {
		if sequences[role] == 0 {
			continue
		}
		producers = append(producers, bundle.Producer{Role: role, InstanceID: "synthetic-" + role, Version: "7.85.0-fixture", Commit: strings.Repeat("b", 40), ProtocolVersion: tc.ProtocolVersion, Streams: producerStreams[role], StopOffset: captureDuration, FinalSequence: sequences[role], AcknowledgedSequence: sequences[role], Stopped: true})
	}
	if err := w.SetProducers(producers); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Complete(captureDuration, profile, cadences); err != nil {
		t.Fatal(err)
	}
}
