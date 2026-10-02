// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package live

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	processapi "github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

const evidenceHost = "engineer-laptop"

type evidenceFixture struct {
	e         *Evidence
	session   Session
	sequences map[string]uint64
}

func newEvidenceFixture(t *testing.T, platform string, connections ...bool) *evidenceFixture {
	t.Helper()
	origin := time.Unix(1700000000, 125000000)
	session := Session{ID: "evidence-session-token", Origin: origin, Duration: time.Minute}
	if len(connections) > 1 && connections[1] {
		session.Duration = 8 * time.Minute
	}
	for _, role := range []string{"core-agent", "process-agent", "system-probe"} {
		streams := []tc.Stream{tc.Metrics, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.Software}
		if role == "process-agent" {
			streams = []tc.Stream{tc.Processes}
			if len(connections) > 0 && connections[0] {
				streams = append(streams, tc.Connections)
			}
		}
		if role == "system-probe" {
			if platform != "windows" {
				continue
			}
			streams = []tc.Stream{tc.Connections}
		}
		if role == "core-agent" && len(connections) > 2 && connections[2] {
			streams = append(streams, tc.HostSystemInfo)
		}
		status := tc.Status{ProtocolVersion: tc.ProtocolVersion, Producer: tc.Identity{Role: role, InstanceID: "evidence-" + role, Version: "7.82.1-producer", Commit: strings.Repeat("b", 40)},
			SessionID: session.ID, State: tc.Active, ActivatedAt: origin}
		if role == "core-agent" {
			status.ActivatedAt = origin.Add(-250 * time.Millisecond)
		}
		for _, stream := range streams {
			capability := tc.Capability{Stream: stream, Cadence: 17 * time.Second}
			if stream == tc.Metrics {
				capability.MetricSchedules = []tc.MetricSchedule{{Family: "cpu", Cadence: 17 * time.Second}}
				if len(connections) > 1 && connections[1] {
					capability.MetricSchedules = append(capability.MetricSchedules, tc.MetricSchedule{Family: "battery", Cadence: 5 * time.Minute})
				}
			}
			status.Capabilities = append(status.Capabilities, capability)
		}
		session.Participants = append(session.Participants, Participant{Status: status, Streams: streams})
	}
	e := NewEvidence(filepath.Join(t.TempDir(), "capture"), platform, "amd64", bundle.BuildIdentity{Version: "7.90.0-tool", Commit: strings.Repeat("a", 40)})
	t.Cleanup(e.Close)
	if err := e.Start(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	return &evidenceFixture{e, session, map[string]uint64{}}
}

func (f *evidenceFixture) record(t *testing.T, stream tc.Stream, offset time.Duration) tc.Record {
	t.Helper()
	var participant Participant
	for _, p := range f.session.Participants {
		if slices.Contains(p.Streams, stream) {
			participant = p
		}
	}
	producer := participant.Status.Producer
	f.sequences[producer.InstanceID]++
	at := f.session.Origin.Add(offset)
	r := tc.Record{ProtocolVersion: tc.ProtocolVersion, SessionID: f.session.ID, Producer: producer, Stream: stream, Sequence: f.sequences[producer.InstanceID], CycleID: f.sequences[producer.InstanceID],
		CollectedAt: at, ObservedAt: at.Add(time.Millisecond), Cadence: 17 * time.Second}
	switch stream {
	case tc.Metrics:
		r.Payload.Series = []tc.Series{{Name: "system.cpu.user", Source: uint32(metrics.MetricSourceCPU), Type: int32(metrics.APIGaugeType), Interval: 15,
			Host: evidenceHost, Device: evidenceHost, Unit: "percent", SourceTypeName: "system", NoIndex: true, Resources: []tc.Resource{{Type: "host", Name: evidenceHost}}, Tags: []string{"interface:" + evidenceHost, "team:engineering", "core:cpu0"},
			Points: []tc.Point{{Timestamp: float64(at.Add(-125*time.Millisecond).UnixNano()) / 1e9, Value: 5.25}, {Timestamp: float64(at.Add(375*time.Millisecond).UnixNano()) / 1e9, Value: 6.5}}}}
	case tc.Metadata:
		osname := "windows"
		if f.e.profile.OS == "macos" {
			osname = "darwin"
		}
		r.Payload.Metadata = &tc.HostMetadata{PythonVersion: "3.13.1", PythonRuntimeVersion: "3.13.1-final", Processor: "Intel64", PublicIPv4: "203.0.113.10",
			Meta:          &tc.HostIdentityMetadata{Hostname: evidenceHost, SocketHostname: evidenceHost, HostAliases: []string{"workstation-alias"}},
			ContainerMeta: map[string]string{"cri_name": "containerd"}, Proxy: &tc.HostProxyMetadata{ProxyBehaviorChanged: true},
			Logs: &tc.HostLogsMetadata{Transport: "HTTP", AutoMultilineEnabled: true}, OTLPEnabled: true, FIPSMode: true, AgentVersion: producer.Version, UUID: evidenceHost, Hostname: evidenceHost, OS: osname, AgentFlavor: "agent", CPUCores: 8, Machine: "amd64", Platform: osname,
			NetworkID: evidenceHost, Gohai: `{"platform":{"hostname":"engineer-laptop","hardware_uuid":"engineer-laptop","serial_number":"DEVICE12345","machine":"amd64"},"network":{"ipaddress":"192.0.2.21","macaddress":"00:01:02:03:04:05","interfaces":[{"name":"en0","ipv4":["192.0.2.21"]}]},"filesystem":[{"name":"/dev/disk3s1","size":1000000}]}`}
	case tc.AgentInventory:
		r.Payload.Inventory = &tc.Inventory{Hostname: evidenceHost, UUID: evidenceHost, Timestamp: at.UnixNano(), Agent: &tc.AgentInventoryMetadata{
			AgentVersion: producer.Version, PackageVersion: producer.Version, Flavor: "agent", InfrastructureMode: "end_user_device",
			AgentStartupTimeMS: f.session.Origin.Add(-time.Hour).UnixMilli(), FeatureProcessEnabled: true, FeatureNetworksEnabled: f.e.profile.OS == "windows",
		}}
	case tc.HostSystemInfo:
		r.Cadence = time.Hour
		r.Payload.Inventory = &tc.Inventory{Hostname: evidenceHost, UUID: evidenceHost, Timestamp: at.UnixNano(), SystemInfo: &tc.HostSystemInfoMetadata{Manufacturer: "Apple Inc.", ModelName: "MacBook Pro", ModelNumber: "Mac16,6", SerialNumber: evidenceHost, Identifier: "Mac16,6", ChassisType: "Laptop"}}
	case tc.HostInventory:
		osname := "Windows"
		if f.e.profile.OS == "macos" {
			osname = "Darwin"
		}
		r.Payload.Inventory = &tc.Inventory{Hostname: evidenceHost, UUID: evidenceHost, Timestamp: at.UnixNano(), Host: &tc.HostInventoryMetadata{
			AgentVersion: producer.Version, OS: osname, KernelName: osname, CPUArchitecture: "amd64", CPUCores: 8, CPULogicalProcessors: 8,
			MemoryTotalKb: 8 << 20, CPUModel: evidenceHost, CPUVendor: evidenceHost, IPAddress: "192.0.2.21", MacAddress: "00:01:02:03:04:05",
		}}
	case tc.Software:
		native := &softwareimpl.Payload{Hostname: evidenceHost, Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "Google Chrome", Version: "125.0.1", Publisher: evidenceHost, Source: "os", ProductCode: evidenceHost, UserSID: evidenceHost, InstallPaths: []string{evidenceHost}}, {DisplayName: evidenceHost, Version: "1.2.3", Source: "os"}}}}
		body, err := native.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		r.Payload.Software = &tc.Message{Body: body, Timestamp: at.UnixNano()}
	case tc.Processes, tc.Connections:
		for i := 0; i < 2; i++ {
			var message model.MessageBody
			if stream == tc.Processes {
				osname := "windows"
				if f.e.profile.OS == "macos" {
					osname = "darwin"
				}
				body := &model.CollectorProc{Hints: &model.CollectorProc_HintMask{HintMask: 1}, HostName: evidenceHost, GroupId: 27, GroupSize: 2, Info: &model.SystemInfo{TotalMemory: 8 << 30, Uuid: evidenceHost, Os: &model.OSInfo{Name: osname}}}
				if i == 1 {
					body.Processes = []*model.Process{{Pid: 42, CreateTime: f.session.Origin.Add(-5 * time.Second).UnixMilli(), Command: &model.Command{Comm: evidenceHost, Exe: evidenceHost, Args: []string{evidenceHost}}, User: &model.ProcessUser{Name: evidenceHost}}}
				}
				message = body
			} else {
				body := &model.CollectorConnections{HostName: evidenceHost, GroupId: 28, GroupSize: 2}
				if i == 1 {
					body.Connections = []*model.Connection{{Pid: 42, Laddr: &model.Addr{Ip: "192.0.2.21", Port: 1234}, Raddr: &model.Addr{Ip: "192.0.2.22", Port: 443}, Rtt: 1250}}
				}
				message = body
			}
			body, err := processapi.EncodePayload(message)
			if err != nil {
				t.Fatal(err)
			}
			r.Payload.Chunks = append(r.Payload.Chunks, tc.Chunk{Body: body, Headers: map[string]string{headers.HostHeader: evidenceHost, headers.RequestIDHeader: strconv.Itoa((27 << 14) + i)}})
		}
	}
	return r
}

func (f *evidenceFixture) accept(ctx context.Context, t *testing.T, record tc.Record) {
	t.Helper()
	if err := f.e.Accept(ctx, record); err != nil {
		t.Fatal(err)
	}

}

func (f *evidenceFixture) stops() []tc.Status {
	var statuses []tc.Status
	for _, participant := range f.session.Participants {
		status := participant.Status
		status.State, status.StoppedAt = tc.Stopped, f.session.Origin.Add(f.session.Duration)
		status.FinalSequence, status.Acknowledged = f.sequences[status.Producer.InstanceID], f.sequences[status.Producer.InstanceID]
		statuses = append(statuses, status)
	}
	return statuses
}

func TestEvidencePersistsNativeSemanticSamples(t *testing.T) {
	f := newEvidenceFixture(t, "windows")
	ctx := context.Background()
	for _, offset := range []time.Duration{time.Second, 16 * time.Second} {
		for _, stream := range []tc.Stream{tc.Metrics, tc.Processes, tc.Connections} {
			f.accept(ctx, t, f.record(t, stream, offset))
		}
	}
	f.accept(ctx, t, f.record(t, tc.Metadata, time.Second))
	for _, stream := range []tc.Stream{tc.AgentInventory, tc.HostInventory} {
		f.accept(ctx, t, f.record(t, stream, 1500*time.Millisecond))
	}
	f.accept(ctx, t, f.record(t, tc.Software, 2375*time.Millisecond))
	if ok, detail := f.e.Coverage(); !ok {
		t.Fatal(detail)
	}
	before, err := os.ReadDir(f.e.directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range []tc.Stream{tc.Metrics, tc.Processes, tc.Connections, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.Software} {
		late := f.record(t, stream, 30*time.Second)
		late.Cadence = time.Hour
		f.accept(ctx, t, late)
	}
	after, err := os.ReadDir(f.e.directory)
	if err != nil || len(after) != len(before)+9 {
		t.Fatal("all later complete stream cycles must extend the bundle")
	}
	// Cleanup may finish later without changing the requested recording window.
	stops := f.stops()
	stops[0].StoppedAt = stops[0].StoppedAt.Add(time.Second)
	if err := f.e.Finish(ctx, stops, f.session.Origin.Add(time.Minute+2*time.Second)); err != nil {
		t.Fatal(err)
	}
	loaded, err := bundle.Load(f.e.directory, f.e.tool.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Manifest.Duration != f.session.Duration || loaded.Manifest.Producers[0].StopOffset <= loaded.Manifest.Duration || loaded.Manifest.Producers[0].StartOffset != -250*time.Millisecond {
		t.Fatal("cleanup changed the recorded duration or lost the later stop boundary")
	}
	for _, stream := range []schema.Stream{schema.Metrics, schema.Processes, schema.Connections} {
		if loaded.Manifest.Cadences[stream] != 15*time.Second {
			t.Fatal("did not derive cadence from distinct collected cycles")
		}
	}
	if loaded.Manifest.Cadences[schema.Software] != time.Hour || loaded.Manifest.Cadences[schema.HostMetadata] != time.Hour {
		t.Fatal("latest producer schedules lost")
	}
	var processPID, connectionPID int32
	for _, ref := range loaded.Manifest.Samples {
		sample, err := loaded.Decode(ref)
		if err != nil {
			t.Fatal(err)
		}
		if sample.Metrics != nil {
			serie := sample.Metrics[0]
			if serie.Name != "system.cpu.user" || serie.Source != metrics.MetricSourceCPU || serie.MType != metrics.APIGaugeType || serie.Interval != 15 ||
				serie.Unit != "percent" || serie.SourceTypeName != "system" || !serie.NoIndex || len(serie.Resources) != 1 || serie.Resources[0].Name != evidenceHost ||
				len(serie.Points) != 2 || serie.Points[0].Ts != ref.Offset.Seconds()-0.125 || serie.Points[1].Ts != ref.Offset.Seconds()+0.375 ||
				serie.Points[0].Value != 5.25 || serie.Points[1].Value != 6.5 {
				t.Fatal("typed metric lost its native fields or fractional relative times")
			}
		}
		if sample.HostMetadata != nil && (sample.HostMetadata.AgentVersion != "7.82.1-producer" || sample.HostMetadata.Hostname != evidenceHost || sample.HostMetadata.UUID != evidenceHost) {
			t.Fatal("capture tool replaced producer identity or version")
		}
		if host := sample.HostMetadata; host != nil {
			if host.PythonVersion != "3.13.1" || host.Network["public-ipv4"] != "203.0.113.10" || string(host.SystemStats["processor"]) != `"Intel64"` ||
				string(host.Meta["host_aliases"]) != `["workstation-alias"]` || host.Logs == nil || host.Logs.Transport != "HTTP" || !host.OTLP["enabled"] || !host.FIPSMode ||
				host.ContainerMeta["cri_name"] != "containerd" || host.Proxy == nil || !host.Proxy.ProxyBehaviorChanged {
				t.Fatal("legacy host projection discarded observed fields")
			}
			if !strings.Contains(host.Gohai, `"interfaces":[{"name":"en0","ipv4":["192.0.2.21"]}]`) || !strings.Contains(host.Gohai, `"filesystem":[{"name":"/dev/disk3s1","size":1000000}]`) {
				t.Fatal("gohai projection changed nested native fields")
			}
		}
		if sample.Software != nil && ((ref.Offset != 2375*time.Millisecond && ref.Offset != 30*time.Second) || sample.Software.Hostname != evidenceHost || sample.Software.Metadata.Software[0].DisplayName != "Google Chrome" || sample.Software.Metadata.Software[1].DisplayName != evidenceHost) {
			t.Fatal("software message lost its fractional relative timestamp or known application")
		}
		if sample.Processes != nil && (sample.Processes.Hints == nil || sample.Processes.GetHintMask() != 1) {
			t.Fatal("native process hints lost")
		}
		if sample.Processes != nil && len(sample.Processes.Processes) > 0 {
			processPID = sample.Processes.Processes[0].Pid
			if sample.Processes.Processes[0].CreateTime != -5000 || sample.Processes.Processes[0].Command.Args[0] != evidenceHost || sample.Processes.Processes[0].User.Name != evidenceHost {
				t.Fatal("process creation time lost")
			}
		}
		if sample.Connections != nil && len(sample.Connections.Connections) > 0 {
			connectionPID = sample.Connections.Connections[0].Pid
			if sample.Connections.Connections[0].Rtt != 1250 {
				t.Fatal("connection round-trip time lost")
			}
		}
	}
	if processPID == 0 || processPID != connectionPID {
		t.Fatal("native cross-stream process identity changed")
	}
	if processPID != 42 {
		t.Fatal("capture replaced the native process ID")
	}
	if len(loaded.Files) != len(loaded.Manifest.Samples) {
		t.Fatal("capture persisted files beyond its typed samples")
	}
	for _, data := range loaded.Files {
		if !bytes.Contains(data, []byte(evidenceHost)) {
			t.Fatal("native hostname missing from captured sample")
		}
	}

}

func TestEvidenceValidatesGroupsAfterCoverageAndIgnoresEmptyGroups(t *testing.T) {
	f := newEvidenceFixture(t, "macos")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	empty := f.record(t, tc.Processes, time.Second)
	for i := range empty.Payload.Chunks {
		message, err := model.DecodeMessage(empty.Payload.Chunks[i].Body)
		if err != nil {
			t.Fatal(err)
		}
		body := message.Body.(*model.CollectorProc)
		body.Processes = nil
		empty.Payload.Chunks[i].Body, err = processapi.EncodePayload(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	f.accept(ctx, t, empty)
	if len(f.e.offsets[schema.Processes]) != 0 {
		t.Fatal("empty complete group counted toward coverage")
	}
	for _, offset := range []time.Duration{2 * time.Second, 17 * time.Second} {
		f.accept(ctx, t, f.record(t, tc.Processes, offset))
	}
	late := f.record(t, tc.Processes, 32*time.Second)
	late.Payload.Chunks = late.Payload.Chunks[:1]
	if err := f.e.Accept(ctx, late); err == nil {
		t.Fatal("covered stream bypassed later group validation")
	}
}

func TestEvidenceCoverageRequiresDistinctTimesAndSafeCompletion(t *testing.T) {
	f := newEvidenceFixture(t, "macos")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for range 2 {
		for _, stream := range []tc.Stream{tc.Metrics, tc.Processes} {
			f.accept(ctx, t, f.record(t, stream, time.Second))
		}
	}
	f.accept(ctx, t, f.record(t, tc.Metadata, time.Second))
	for _, stream := range []tc.Stream{tc.AgentInventory, tc.HostInventory} {
		f.accept(ctx, t, f.record(t, stream, 1500*time.Millisecond))
	}
	f.accept(ctx, t, f.record(t, tc.Software, 2*time.Second))
	if ok, _ := f.e.Coverage(); ok {
		t.Fatal("equal collection offsets invented cadence")
	}
	for _, stream := range []tc.Stream{tc.Metrics, tc.Processes} {
		f.accept(ctx, t, f.record(t, stream, 16*time.Second))
	}
	if ok, detail := f.e.Coverage(); !ok {
		t.Fatal(detail)
	}
	stops := f.stops()
	stops[0].Acknowledged--
	if err := f.e.Finish(ctx, stops, f.session.Origin.Add(time.Minute)); err == nil {
		t.Fatal("accepted missing stop acknowledgement")
	}
	if _, err := os.Stat(filepath.Join(f.e.directory, "COMPLETE")); !os.IsNotExist(err) {
		t.Fatal("failed capture became complete")
	}
}

func TestEvidenceRejectsPartialAndForeignGroupsWithoutCompletion(t *testing.T) {
	for _, kind := range []string{"partial", "reordered", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			f := newEvidenceFixture(t, "macos")
			r := f.record(t, tc.Processes, time.Second)
			switch kind {
			case "partial":
				r.Payload.Chunks = r.Payload.Chunks[:1]
			case "reordered":
				r.Payload.Chunks[0], r.Payload.Chunks[1] = r.Payload.Chunks[1], r.Payload.Chunks[0]
			case "foreign":
				r.Producer.InstanceID = "foreign-producer"
			}
			if err := f.e.Accept(context.Background(), r); err == nil {
				t.Fatal("accepted invalid group")
			}
			if _, err := os.Stat(filepath.Join(f.e.directory, "COMPLETE")); !os.IsNotExist(err) {
				t.Fatal("invalid group became complete")
			}
		})
	}
}

func TestEvidenceWaitsForSlowMetricFamiliesAndCapturesMacOSConnections(t *testing.T) {
	f := newEvidenceFixture(t, "macos", true, true, true)
	ctx := context.Background()
	for _, offset := range []time.Duration{time.Second, 16 * time.Second} {
		for _, stream := range []tc.Stream{tc.Metrics, tc.Processes, tc.Connections} {
			f.accept(ctx, t, f.record(t, stream, offset))
		}
	}
	f.accept(ctx, t, f.record(t, tc.Metadata, time.Second))
	for _, stream := range []tc.Stream{tc.AgentInventory, tc.HostInventory} {
		f.accept(ctx, t, f.record(t, stream, time.Second))
	}
	f.accept(ctx, t, f.record(t, tc.Software, time.Second))
	if ok, detail := f.e.Coverage(); ok || !strings.Contains(detail, "metrics/battery: 0/2") || !strings.Contains(detail, "5m0s") {
		t.Fatalf("slow scheduled family did not prevent false completion: %v %s", ok, detail)
	}
	if _, detail := f.e.Coverage(); !strings.Contains(detail, "host_system_info: 0/1") {
		t.Fatal("selected hardware stream not required for completeness")
	}
	f.accept(ctx, t, f.record(t, tc.HostSystemInfo, 3*time.Second))
	for i, offset := range []time.Duration{2 * time.Minute, 7 * time.Minute} {
		r := f.record(t, tc.Metrics, offset)
		r.Payload.Series[0].Name = "system.battery.current_charge_pct"
		f.accept(ctx, t, r)
		if ok, _ := f.e.Coverage(); ok != (i == 1) {
			t.Fatal("battery coverage did not count distinct late observations")
		}
	}
	if len(f.e.offsets[schema.Metrics]) != 4 || len(f.e.offsets[schema.Connections]) != 2 {
		t.Fatal("late metrics or optional macOS connection groups were discarded")
	}
	stops := f.stops()
	for i := range stops {
		stops[i].StoppedAt = f.session.Origin.Add(8 * time.Minute)
	}
	if err := f.e.Finish(ctx, stops, f.session.Origin.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	loaded, err := bundle.Load(f.e.directory, f.e.tool.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Manifest.MetricCadences["battery"] != 5*time.Minute || !slices.Contains(loaded.Manifest.Profile.Streams, schema.Connections) || loaded.Manifest.Cadences[schema.HostSystemInfo] != time.Hour {
		t.Fatal("late battery or macOS connection evidence missing")
	}
}

func TestEvidenceWindowKeepsWholeGroupsAndEveryCycle(t *testing.T) {
	f := newEvidenceFixture(t, "windows")
	ctx := context.Background()
	for _, stream := range []tc.Stream{tc.Metrics, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.Software, tc.Processes, tc.Connections} {
		// A cycle collected before all producers activated is excluded even if
		// its delivery was observed inside the recording window.
		before := f.record(t, stream, -time.Nanosecond)
		before.ObservedAt = f.session.Origin
		f.accept(ctx, t, before)
		// The end is exclusive: collection may start before it but must be
		// observed before it for the entire logical item to be retained.
		boundary := f.record(t, stream, f.session.Duration-time.Nanosecond)
		boundary.ObservedAt = f.session.Origin.Add(f.session.Duration)
		f.accept(ctx, t, boundary)
		if len(f.e.offsets[evidenceStream(stream)]) != 0 {
			t.Fatal("outside-window observation was retained")
		}
		for _, offset := range []time.Duration{0, time.Second, time.Second, 2 * time.Second} {
			f.accept(ctx, t, f.record(t, stream, offset))
		}
		if len(f.e.offsets[evidenceStream(stream)]) != 4 {
			t.Fatal("distinct cycles were dropped after coverage or at equal offsets")
		}
	}
	if err := f.e.Finish(ctx, f.stops(), f.session.Origin.Add(f.session.Duration)); err != nil {
		t.Fatal(err)
	}
	loaded, err := bundle.Load(f.e.directory, f.e.tool.Commit)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[schema.Stream]int{}
	for _, sample := range loaded.Manifest.Samples {
		counts[sample.Stream]++
	}
	for stream, count := range counts {
		want := 4
		if stream == schema.Processes || stream == schema.Connections {
			want = 8
		}
		if count != want {
			t.Fatalf("%s retained %d chunks; want %d", stream, count, want)
		}
	}
}
