// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package live

import (
	"bytes"
	"context"
	"encoding/json"
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

const evidenceSecret = "UNIQUE-NATIVE-IDENTITY-OR-CREDENTIAL"

type evidenceFixture struct {
	e         *Evidence
	session   Session
	sequences map[string]uint64
}

func newEvidenceFixture(t *testing.T, platform string, connections ...bool) *evidenceFixture {
	t.Helper()
	origin := time.Unix(1700000000, 125000000)
	session := Session{ID: "evidence-session-token", Origin: origin}
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
			Host: evidenceSecret, Device: evidenceSecret, Tags: []string{"interface:" + evidenceSecret, "secret:" + evidenceSecret, "core:cpu0"},
			Points: []tc.Point{{Timestamp: float64(at.Add(-125*time.Millisecond).UnixNano()) / 1e9, Value: 5.25}, {Timestamp: float64(at.Add(375*time.Millisecond).UnixNano()) / 1e9, Value: 6.5}}}}
	case tc.Metadata:
		osname := "windows"
		if f.e.profile.OS == "macos" {
			osname = "darwin"
		}
		r.Payload.Metadata = &tc.HostMetadata{AgentVersion: producer.Version, UUID: evidenceSecret, Hostname: evidenceSecret, OS: osname, AgentFlavor: "agent", CPUCores: 8, Machine: "amd64", Platform: osname,
			NetworkID: evidenceSecret, Gohai: map[string]map[string]string{"platform": {"hostname": evidenceSecret, "hardware_uuid": evidenceSecret, "serial_number": evidenceSecret, "machine": "amd64"}, "network": {"ipaddress": "192.0.2.21", "macaddress": "00:01:02:03:04:05"}}}
	case tc.AgentInventory:
		r.Payload.Inventory = &tc.Inventory{Hostname: evidenceSecret, UUID: evidenceSecret, Timestamp: at.UnixNano(), Agent: &tc.AgentInventoryMetadata{
			AgentVersion: producer.Version, PackageVersion: producer.Version, Flavor: "agent", InfrastructureMode: "end_user_device",
			AgentStartupTimeMS: f.session.Origin.Add(-time.Hour).UnixMilli(), FeatureProcessEnabled: true, FeatureNetworksEnabled: f.e.profile.OS == "windows",
		}}
	case tc.HostSystemInfo:
		r.Cadence = time.Hour
		r.Payload.Inventory = &tc.Inventory{Hostname: evidenceSecret, UUID: evidenceSecret, Timestamp: at.UnixNano(), SystemInfo: &tc.HostSystemInfoMetadata{Manufacturer: "Apple Inc.", ModelName: "MacBook Pro", ModelNumber: "Mac16,6", SerialNumber: evidenceSecret, Identifier: "Mac16,6", ChassisType: "Laptop"}}
	case tc.HostInventory:
		osname := "Windows"
		if f.e.profile.OS == "macos" {
			osname = "Darwin"
		}
		r.Payload.Inventory = &tc.Inventory{Hostname: evidenceSecret, UUID: evidenceSecret, Timestamp: at.UnixNano(), Host: &tc.HostInventoryMetadata{
			AgentVersion: producer.Version, OS: osname, KernelName: osname, CPUArchitecture: "amd64", CPUCores: 8, CPULogicalProcessors: 8,
			MemoryTotalKb: 8 << 20, CPUModel: evidenceSecret, CPUVendor: evidenceSecret, IPAddress: "192.0.2.21", MacAddress: "00:01:02:03:04:05",
		}}
	case tc.Software:
		native := &softwareimpl.Payload{Hostname: evidenceSecret, Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "Google Chrome", Version: "125.0.1", Publisher: evidenceSecret, Source: "os", ProductCode: evidenceSecret, UserSID: evidenceSecret, InstallPaths: []string{evidenceSecret}}, {DisplayName: evidenceSecret, Version: "1.2.3", Source: "os"}}}}
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
				body := &model.CollectorProc{HostName: evidenceSecret, GroupId: 27, GroupSize: 2, Info: &model.SystemInfo{TotalMemory: 8 << 30, Uuid: evidenceSecret, Os: &model.OSInfo{Name: osname}}}
				if i == 1 {
					body.Processes = []*model.Process{{Pid: 42, CreateTime: f.session.Origin.Add(-5 * time.Second).UnixMilli(), Command: &model.Command{Comm: evidenceSecret, Exe: evidenceSecret, Args: []string{evidenceSecret}}, User: &model.ProcessUser{Name: evidenceSecret}}}
				}
				message = body
			} else {
				body := &model.CollectorConnections{HostName: evidenceSecret, GroupId: 28, GroupSize: 2}
				if i == 1 {
					body.Connections = []*model.Connection{{Pid: 42, Laddr: &model.Addr{Ip: "192.0.2.21", Port: 1234}, Raddr: &model.Addr{Ip: "192.0.2.22", Port: 443}, Rtt: 1250}}
				}
				message = body
			}
			body, err := processapi.EncodePayload(message)
			if err != nil {
				t.Fatal(err)
			}
			r.Payload.Chunks = append(r.Payload.Chunks, tc.Chunk{Body: body, Headers: map[string]string{headers.HostHeader: evidenceSecret, headers.RequestIDHeader: strconv.Itoa((27 << 14) + i)}})
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
		status.State, status.StoppedAt = tc.Stopped, f.session.Origin.Add(time.Minute)
		status.FinalSequence, status.Acknowledged = f.sequences[status.Producer.InstanceID], f.sequences[status.Producer.InstanceID]
		statuses = append(statuses, status)
	}
	return statuses
}

func TestEvidencePersistsSanitizedSemanticSamples(t *testing.T) {
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
	if err != nil || len(after) != len(before)+1 {
		t.Fatal("only the later typed metric cycle should extend the bundle")
	}
	if err := f.e.Finish(ctx, f.stops(), f.session.Origin.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	loaded, err := bundle.Load(f.e.directory, f.e.tool.Commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range []schema.Stream{schema.Metrics, schema.Processes, schema.Connections} {
		if loaded.Manifest.Cadences[stream] != 15*time.Second {
			t.Fatal("did not derive cadence from distinct collected cycles")
		}
	}
	if loaded.Manifest.Cadences[schema.Software] != 17*time.Second || loaded.Manifest.Cadences[schema.HostMetadata] != 17*time.Second {
		t.Fatal("singleton schedule lost")
	}
	var processPID, connectionPID int32
	for _, ref := range loaded.Manifest.Samples {
		sample := loaded.Samples[ref.File]
		if sample.Metrics != nil {
			serie := sample.Metrics[0]
			if serie.Name != "system.cpu.user" || serie.Source != metrics.MetricSourceCPU || serie.MType != metrics.APIGaugeType || serie.Interval != 15 ||
				len(serie.Points) != 2 || serie.Points[0].Ts != ref.Offset.Seconds()-0.125 || serie.Points[1].Ts != ref.Offset.Seconds()+0.375 ||
				serie.Points[0].Value != 5.25 || serie.Points[1].Value != 6.5 {
				t.Fatal("typed metric lost its name, source, type, interval, values or fractional relative times")
			}
		}
		if sample.HostMetadata != nil && sample.HostMetadata.AgentVersion != "7.82.1-producer" {
			t.Fatal("capture tool replaced producer version")
		}
		if sample.Software != nil && (ref.Offset != 2375*time.Millisecond || sample.Software.Metadata.Software[0].DisplayName != "Google Chrome") {
			t.Fatal("software message lost its fractional relative timestamp or known application")
		}
		if sample.Processes != nil && len(sample.Processes.Processes) > 0 {
			processPID = sample.Processes.Processes[0].Pid
			if sample.Processes.Processes[0].CreateTime != -5000 {
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
		t.Fatal("sanitizer did not preserve cross-stream process identity")
	}
	manifest, _ := json.Marshal(loaded.Manifest)
	if bytes.Contains(manifest, []byte(evidenceSecret)) {
		t.Fatal("native identity persisted in manifest")
	}
	if len(loaded.Files) != len(loaded.Manifest.Samples) {
		t.Fatal("capture persisted files beyond its typed samples")
	}
	for _, data := range loaded.Files {
		if bytes.Contains(data, []byte(evidenceSecret)) {
			t.Fatal("native identity persisted in typed file")
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

func TestEvidenceDiscardedGroupsDoNotRetainNewIdentities(t *testing.T) {
	f := newEvidenceFixture(t, "windows")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, stream := range []tc.Stream{tc.Processes, tc.Connections} {
		for _, offset := range []time.Duration{time.Second, 16 * time.Second} {
			f.accept(ctx, t, f.record(t, stream, offset))
		}
		omitted := f.record(t, stream, 32*time.Second)
		for i := range omitted.Payload.Chunks {
			message, err := model.DecodeMessage(omitted.Payload.Chunks[i].Body)
			if err != nil {
				t.Fatal(err)
			}
			switch body := message.Body.(type) {
			case *model.CollectorProc:
				for _, process := range body.Processes {
					process.Pid = 123456
				}
			case *model.CollectorConnections:
				for _, connection := range body.Connections {
					connection.Pid = 123457
				}
			}
			omitted.Payload.Chunks[i].Body, err = processapi.EncodePayload(message.Body)
			if err != nil {
				t.Fatal(err)
			}
		}
		f.accept(ctx, t, omitted)
	}
	// A control sanitizer that has only seen the retained PID must allocate the
	// same next placeholder. Discarded process/connection identities consume no
	// entries in the live session's cross-stream mapping.
	control := newEvidenceFixture(t, "windows")
	control.e.sanitizer.Process(&model.CollectorProc{Processes: []*model.Process{{Pid: 42}}})
	probe := &model.CollectorProc{Processes: []*model.Process{{Pid: 999999}}}
	want := control.e.sanitizer.Process(probe).Processes[0].Pid
	got := f.e.sanitizer.Process(probe).Processes[0].Pid
	if got != want {
		t.Fatal("discarded cycles retained new native identities")
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
