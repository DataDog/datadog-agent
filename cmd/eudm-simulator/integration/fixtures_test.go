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
)

// Synthetic inputs, serialized by real Agent delivery packages. This fixture
// commit deliberately cannot be used by a revision-stamped staging binary.
const fixtureCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestGenerateCaptureFixtures(t *testing.T) {
	if os.Getenv("EUDM_GENERATE_FIXTURES") != "1" {
		t.Skip("explicit fixture generation only")
	}
	for _, platform := range []string{"macos", "windows"} {
		t.Run(platform, func(t *testing.T) { generateFixture(t, platform) })
	}
}
func generateFixture(t *testing.T, platform string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recorder := output.NewRecorder()
	p, err := output.New(ctx, nil, "synthetic-fixture-no-credential", recorder)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	directory := filepath.Join("..", "testdata", "bundles", platform)
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		t.Fatal(err)
	}
	w, err := bundle.NewWriter(directory, bundle.Manifest{AgentVersion: "7.85.0-fixture", AgentCommit: fixtureCommit})
	if err != nil {
		t.Fatal(err)
	}
	osname, hostOS, arch, chrome := "darwin", "darwin", "arm64", "Google Chrome"
	if platform == "windows" {
		osname, hostOS, arch, chrome = "windows", "win32", "amd64", "chrome.exe"
	}
	background := "process-" + strings.Repeat("1", 32)
	profile := schema.Profile{OS: platform, Architecture: arch, MemoryBytes: 16 << 30, Streams: []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Software}, ProcessNames: []string{chrome, background}, SoftwareNames: []string{"Google Chrome", "OS"}}
	if platform == "windows" {
		profile.Streams = append(profile.Streams, schema.Connections)
		profile.ProcessNames = append(profile.ProcessNames, "SentinelAgent.exe")
		profile.SoftwareNames = append(profile.SoftwareNames, "SentinelOne")
	}
	cadences := map[schema.Stream]time.Duration{schema.Metrics: 15 * time.Second, schema.Processes: 10 * time.Second, schema.HostMetadata: 5 * time.Minute, schema.Software: 10 * time.Minute}
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
	}
	values := map[string]float64{"system.cpu.user": 5, "system.cpu.system": 2, "system.cpu.idle": 93, "system.cpu.num_cores": 4, "system.mem.total": 16384, "system.mem.used": 4096, "system.mem.free": 12288, "system.mem.usable": 12288, "system.mem.pct_usable": 0.75, "system.wlan.rssi": -55, "system.wlan.noise": -95, "system.wlan.txrate": 600, "system.wlan.rxrate": 600, "system.wlan.status": 1}
	for name := range values {
		profile.MetricNames = append(profile.MetricNames, name)
	}
	sort.Strings(profile.MetricNames)
	for cycle := 0; cycle < 2; cycle++ {
		offset := time.Duration(cycle) * 15 * time.Second
		var series []*metrics.Serie
		for _, name := range profile.MetricNames {
			source := metrics.MetricSourceCPU
			tags := []string{"infra_mode:end_user_device"}
			if strings.HasPrefix(name, "system.mem.") {
				source = metrics.MetricSourceMemory
			}
			if strings.HasPrefix(name, "system.wlan.") {
				source = metrics.MetricSourceWlan
				tags = append(tags, "bssid:02:00:00:00:00:01", "mac_address:02:00:00:00:00:02", "ssid:ssid-fixture", "interface:interface-"+strings.Repeat("2", 32))
			}
			series = append(series, &metrics.Serie{Name: name, Host: "capture-host", MType: metrics.APIGaugeType, Source: source, Interval: 15, Tags: tagset.CompositeTagsFromSlice(tags), Points: []metrics.Point{{Ts: offset.Seconds(), Value: values[name]}}})
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
		if platform == "windows" {
			conn := &model.CollectorConnections{HostName: "capture-host", NetworkId: proc.NetworkId, GroupId: int32(cycle + 1), GroupSize: 1, Connections: []*model.Connection{{Pid: 300, Laddr: &model.Addr{Ip: "10.0.0.1", Port: 50000}, Raddr: &model.Addr{Ip: "10.0.0.2", Port: 443}, Type: model.ConnectionType_tcp, Rtt: 20000, RttVar: 2000, LastBytesSent: 1000, LastBytesReceived: 2000}}}
			if cycle == 0 {
				profile.ConnectionSelectors = []string{telemetry.ConnectionSelector(conn.Connections[0])}
			}
			cadences[schema.Connections] = 10 * time.Second
			save(schema.Connections, offset, conn, func() error { return p.Connections(ctx, time.Unix(int64(offset.Seconds()), 0), conn) })
		}
	}
	host := &telemetry.HostMetadata{Hostname: "capture-host", UUID: "00000000-0000-4000-8000-000000000001", AgentVersion: "7.85.0-fixture", AgentFlavor: "agent", OS: hostOS, HostTags: map[string][]string{"system": {"infra_mode:end_user_device"}}}
	save(schema.HostMetadata, 0, host, func() error { return p.Serializer.SendHostMetadata(host) })
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
	if _, err := w.Complete(30*time.Second, profile, cadences); err != nil {
		t.Fatal(err)
	}
}
