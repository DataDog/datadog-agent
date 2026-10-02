// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib && zstd

package live

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
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
	processapi "github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
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
		status := tc.Status{ProtocolVersion: 1, Producer: tc.Identity{Role: role, InstanceID: "evidence-" + role, Version: "7.82.1-producer", Commit: strings.Repeat("b", 40)},
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

func (f *evidenceFixture) record(t *testing.T, stream tc.Stream, offset time.Duration, protocol string) tc.Record {
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
	r := tc.Record{ProtocolVersion: 1, SessionID: f.session.ID, Producer: producer, Stream: stream, Sequence: f.sequences[producer.InstanceID], CycleID: f.sequences[producer.InstanceID],
		CollectedAt: at, ObservedAt: at.Add(time.Millisecond), Cadence: 17 * time.Second}
	if stream == tc.Metrics || stream == tc.Metadata || stream == tc.AgentInventory || stream == tc.HostInventory || stream == tc.HostSystemInfo {
		path := map[string]string{"v1": "/api/v1/series", "v2": "/api/v2/series", "v3": "/api/intake/metrics/v3/series", "v3beta": "/api/intake/metrics/v3beta/series", "metadata-v1": "/intake/", "metadata-v2": "/api/v2/host_metadata", "inventory-v1": "/api/v1/metadata"}[protocol]
		r.Payload.Routes = []tc.Route{{PayloadID: 1, Endpoint: path, Protocol: protocol, Destination: "primary/1", EnqueuedAt: r.ObservedAt}}
	}
	switch stream {
	case tc.Metrics:
		r.Payload.Series = []tc.Series{{Ordinal: 1, Name: "system.cpu.user", Source: uint32(metrics.MetricSourceCPU), Type: int32(metrics.APIGaugeType), Interval: 15,
			Host: evidenceSecret, Device: evidenceSecret, Tags: []string{"interface:" + evidenceSecret, "secret:" + evidenceSecret, "core:cpu0"},
			Points: []tc.Point{{Timestamp: float64(at.Add(-125*time.Millisecond).UnixNano()) / 1e9, Value: 5.25}, {Timestamp: float64(at.Add(375*time.Millisecond).UnixNano()) / 1e9, Value: 6.5}}}}
		r.Payload.Routes[0].Ordinals = []uint64{1}
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
	for _, pipeline := range f.e.pipelines {
		if retained := pipeline.recorder.Drain(); len(retained) != 0 {
			t.Fatal("writer retained prior cycle wire bodies")
		}
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

func TestEvidenceRegeneratesAllObservedWireProtocols(t *testing.T) {
	for i, protocol := range []string{"v1", "v2", "v3", "v3beta"} {
		t.Run(protocol, func(t *testing.T) {
			f := newEvidenceFixture(t, "windows")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, offset := range []time.Duration{time.Second, 16 * time.Second} {
				for _, stream := range []tc.Stream{tc.Metrics, tc.Processes, tc.Connections} {
					f.accept(ctx, t, f.record(t, stream, offset, protocol))
				}
			}
			f.accept(ctx, t, f.record(t, tc.Metadata, time.Second, []string{"metadata-v1", "metadata-v2"}[i%2]))
			for _, stream := range []tc.Stream{tc.AgentInventory, tc.HostInventory} {
				f.accept(ctx, t, f.record(t, stream, 1500*time.Millisecond, "inventory-v1"))
			}
			f.accept(ctx, t, f.record(t, tc.Software, 2375*time.Millisecond, ""))
			if ok, detail := f.e.Coverage(); !ok {
				t.Fatal(detail)
			}
			before, err := os.ReadDir(f.e.directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, stream := range []tc.Stream{tc.Metrics, tc.Processes, tc.Connections, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.Software} {
				observed := protocol
				if stream == tc.Metadata {
					observed = "metadata-v1"
				}
				if stream == tc.AgentInventory || stream == tc.HostInventory {
					observed = "inventory-v1"
				}
				late := f.record(t, stream, 30*time.Second, observed)
				late.Cadence = time.Hour
				f.accept(ctx, t, late)
			}
			after, err := os.ReadDir(f.e.directory)
			if err != nil || len(after) != len(before)+2 {
				t.Fatal("only the later metric cycle should extend the bundle")
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
					if sample.Metrics[0].Source != metrics.MetricSourceCPU || sample.Metrics[0].Points[0].Ts != ref.Offset.Seconds()-0.125 {
						t.Fatal("typed metric lost source or fractional relative time")
					}
				}
				if sample.HostMetadata != nil && sample.HostMetadata.AgentVersion != "7.82.1-producer" {
					t.Fatal("capture tool replaced producer version")
				}
				if sample.Software != nil && ref.Offset != 2375*time.Millisecond {
					t.Fatal("software message lost its fractional relative timestamp")
				}
				if sample.Processes != nil && len(sample.Processes.Processes) > 0 {
					processPID = sample.Processes.Processes[0].Pid
					if sample.Processes.Processes[0].CreateTime != -5000 {
						t.Fatal("process creation time lost")
					}
				}
				if sample.Connections != nil && len(sample.Connections.Connections) > 0 {
					connectionPID = sample.Connections.Connections[0].Pid
				}
				for _, name := range ref.WireFiles {
					var wire bundle.WireReference
					if err := bundle.DecodeJSON(loaded.Files[name], &wire); err != nil {
						t.Fatal(err)
					}
					assertEvidenceWire(t, loaded.Files[ref.File], sample, ref, wire)
				}
			}
			if processPID == 0 || processPID != connectionPID {
				t.Fatal("sanitizer did not preserve cross-stream process identity")
			}
			manifest, _ := json.Marshal(loaded.Manifest)
			if bytes.Contains(manifest, []byte(evidenceSecret)) {
				t.Fatal("native identity persisted in manifest")
			}
			for _, data := range loaded.Files {
				if bytes.Contains(data, []byte(evidenceSecret)) {
					t.Fatal("native identity persisted in typed or wire file")
				}
			}
		})
	}
}

func TestEvidenceValidatesGroupsAfterCoverageAndIgnoresEmptyGroups(t *testing.T) {
	f := newEvidenceFixture(t, "macos")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	empty := f.record(t, tc.Processes, time.Second, "")
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
		f.accept(ctx, t, f.record(t, tc.Processes, offset, ""))
	}
	late := f.record(t, tc.Processes, 32*time.Second, "")
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
			f.accept(ctx, t, f.record(t, stream, offset, ""))
		}
		omitted := f.record(t, stream, 32*time.Second, "")
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

func TestEvidenceBoundsCycleHistory(t *testing.T) {
	f := newEvidenceFixture(t, "macos")
	record := f.record(t, tc.Metrics, time.Second, "v2")
	for i := uint64(1); i <= maxSessionCycles; i++ {
		f.e.cycles[record.Producer.InstanceID][i] = true
	}
	record.CycleID = maxSessionCycles + 1
	if err := f.e.Accept(context.Background(), record); err == nil {
		t.Fatal("capture retained unbounded cycle history")
	}
	if !f.e.closed || len(f.e.cycles) != 0 {
		t.Fatal("failed capture retained its consumed cycle history")
	}
	if _, err := os.Stat(filepath.Join(f.e.directory, "COMPLETE")); !os.IsNotExist(err) {
		t.Fatal("history overflow produced a completion marker")
	}
}

func TestEvidenceMixedProtocolsRegenerateOnlyObservedMembers(t *testing.T) {
	f := newEvidenceFixture(t, "macos")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r := f.record(t, tc.Metrics, time.Second, "v2")
	second := r.Payload.Series[0]
	second.Ordinal, second.Name = 2, "system.cpu.system"
	r.Payload.Series = append(r.Payload.Series, second)
	r.Payload.Routes = append(r.Payload.Routes, tc.Route{PayloadID: 2, Ordinals: []uint64{2}, Endpoint: "/api/intake/metrics/v3/series", Protocol: "v3", Destination: "additional-1/1", EnqueuedAt: r.ObservedAt})
	f.accept(ctx, t, r)
	for i, expected := range []string{"system.cpu.user", "system.cpu.system"} {
		data, err := os.ReadFile(filepath.Join(f.e.directory, fmt.Sprintf("sample-000000-wire-%03d.json", i)))
		if err != nil {
			t.Fatal(err)
		}
		var wire bundle.WireReference
		if err := bundle.DecodeJSON(data, &wire); err != nil {
			t.Fatal(err)
		}
		decoded := decodeMetricWire(t, wire)
		if len(decoded) != 1 || decoded[0].Name != expected {
			t.Fatal("protocol regeneration crossed filtered memberships")
		}
	}
}

func TestEvidenceCoverageRequiresDistinctTimesAndSafeCompletion(t *testing.T) {
	f := newEvidenceFixture(t, "macos")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for range 2 {
		for _, stream := range []tc.Stream{tc.Metrics, tc.Processes} {
			f.accept(ctx, t, f.record(t, stream, time.Second, "v1"))
		}
	}
	f.accept(ctx, t, f.record(t, tc.Metadata, time.Second, "metadata-v1"))
	for _, stream := range []tc.Stream{tc.AgentInventory, tc.HostInventory} {
		f.accept(ctx, t, f.record(t, stream, 1500*time.Millisecond, "inventory-v1"))
	}
	f.accept(ctx, t, f.record(t, tc.Software, 2*time.Second, ""))
	if ok, _ := f.e.Coverage(); ok {
		t.Fatal("equal collection offsets invented cadence")
	}
	for _, stream := range []tc.Stream{tc.Metrics, tc.Processes} {
		f.accept(ctx, t, f.record(t, stream, 16*time.Second, "v1"))
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
	for _, kind := range []string{"partial", "reordered", "foreign", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			f := newEvidenceFixture(t, "macos")
			r := f.record(t, tc.Processes, time.Second, "")
			switch kind {
			case "partial":
				r.Payload.Chunks = r.Payload.Chunks[:1]
			case "reordered":
				r.Payload.Chunks[0], r.Payload.Chunks[1] = r.Payload.Chunks[1], r.Payload.Chunks[0]
			case "foreign":
				r.Producer.InstanceID = "foreign-producer"
			case "duplicate":
				f.accept(context.Background(), t, r)
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

func assertEvidenceWire(t *testing.T, typed []byte, sample *telemetry.Sample, ref bundle.SampleRef, wire bundle.WireReference) {
	t.Helper()
	head, _ := json.Marshal(wire.Headers)
	if bytes.Contains(head, []byte(evidenceSecret)) || wire.Headers.Get("Authorization") != "" || wire.Headers.Get("DD-API-KEY") != "" {
		t.Fatal("unsafe wire headers")
	}
	if sample.Metrics != nil {
		actual := decodeMetricWire(t, wire)
		if len(actual) != len(sample.Metrics) {
			t.Fatal("metric wire membership differs")
		}
		for i, got := range actual {
			want := sample.Metrics[i]
			if got.Name != want.Name || got.Host != want.Host || got.Device != want.Device || got.Type != want.MType.String() || got.Interval != want.Interval || len(got.Points) != len(want.Points) {
				t.Fatalf("metric wire shape differs: %+v", got)
			}
			tags := slices.Clone(want.Tags.UnsafeToReadOnlySliceString())
			slices.Sort(tags)
			slices.Sort(got.Tags)
			if !slices.Equal(tags, got.Tags) {
				t.Fatal("metric tags differ")
			}
			for j, point := range got.Points {
				if point[0] != float64(int64(want.Points[j].Ts)) || point[1] != want.Points[j].Value {
					t.Fatal("representable metric point differs")
				}
			}
		}
		return
	}
	var body []byte
	if sample.Processes != nil || sample.Connections != nil {
		message, err := model.DecodeMessage(wire.Body)
		if err != nil {
			t.Fatal(err)
		}
		body, _ = json.Marshal(message.Body)
		id, err := strconv.ParseUint(wire.Headers.Get(headers.RequestIDHeader), 10, 64)
		if err != nil || int(id&((1<<14)-1)) != ref.ChunkIndex {
			t.Fatal("wire chunk order differs")
		}
	} else {
		body = decompressEvidence(t, wire)
		if sample.Software != nil {
			var batch []json.RawMessage
			if err := json.Unmarshal(body, &batch); err != nil || len(batch) != 1 {
				t.Fatalf("software batching differs: %v", err)
			}
			body = batch[0]
		}
	}
	if bytes.Contains(body, []byte(evidenceSecret)) {
		t.Fatal("native identity in decoded wire")
	}
	var want, got any
	if err := json.Unmarshal(typed, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("sanitized %s sample differs from decoded wire", ref.Stream)
	}
}

func decompressEvidence(t *testing.T, wire bundle.WireReference) []byte {
	t.Helper()
	encoding := wire.Headers.Get("Content-Encoding")
	if encoding == "" || encoding == "identity" {
		return wire.Body
	}
	kind := map[string]string{"deflate": "zlib", "gzip": "gzip", "zstd": "zstd"}[encoding]
	if kind == "" {
		t.Fatal("unsupported wire encoding")
	}
	data, err := selector.NewCompressor(kind, 1).Decompress(wire.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(evidenceSecret)) {
		t.Fatal("native identity in decoded body")
	}
	return data
}

type wireMetric struct {
	Name     string       `json:"metric"`
	Host     string       `json:"host"`
	Device   string       `json:"device"`
	Tags     []string     `json:"tags"`
	Type     string       `json:"type"`
	Interval int64        `json:"interval"`
	Points   [][2]float64 `json:"points"`
}

// Decode the actual wire representations independently of the serializer. v2
// and v3 carry source-derived origin metadata, not the exact source enum;
// timestamps in all three formats are integer seconds.
func decodeMetricWire(t *testing.T, wire bundle.WireReference) []wireMetric {
	t.Helper()
	data := decompressEvidence(t, wire)
	if wire.Path == "/api/v1/series" {
		var value struct {
			Series []wireMetric `json:"series"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value.Series
	}
	fields := protoFields(t, data)
	if wire.Path == "/api/v2/series" {
		var result []wireMetric
		for _, raw := range fields[1] {
			f := protoFields(t, raw)
			m := wireMetric{Name: string(first(f[2])), Type: map[uint64]string{1: "count", 2: "rate", 3: "gauge"}[fieldNumber(f[5])], Interval: int64(fieldNumber(f[8]))}
			for _, tag := range f[3] {
				m.Tags = append(m.Tags, string(tag))
			}
			for _, resource := range f[1] {
				r := protoFields(t, resource)
				switch string(first(r[1])) {
				case "host":
					m.Host = string(first(r[2]))
				case "device":
					m.Device = string(first(r[2]))
				}
			}
			for _, raw := range f[4] {
				p := protoFields(t, raw)
				m.Points = append(m.Points, [2]float64{float64(int64(fieldNumber(p[2]))), math.Float64frombits(binary.LittleEndian.Uint64(first(p[1])))})
			}
			result = append(result, m)
		}
		return result
	}
	columns := protoFields(t, first(fields[3]))
	c := map[int][]byte{}
	for id, v := range columns {
		c[id] = first(v)
	}
	stringsAt := func(id int) []string {
		v := []string{""}
		data := c[id]
		for len(data) > 0 {
			n := takeUvarint(t, &data)
			if n > uint64(len(data)) {
				t.Fatal("invalid v3 dictionary")
			}
			v = append(v, string(data[:n]))
			data = data[n:]
		}
		return v
	}
	names, tags, resources := stringsAt(1), stringsAt(2), stringsAt(4)
	tagsets := [][]string{nil}
	for len(c[3]) > 0 {
		n := takeSint(t, &c, 3)
		var values []string
		var index int64
		for range n {
			index += takeSint(t, &c, 3)
			if index < 0 {
				values = append(values, tagsets[-index]...)
			} else {
				values = append(values, tags[index])
			}
		}
		tagsets = append(tagsets, values)
	}
	resourceSets := []map[string]string{nil}
	for len(c[5]) > 0 {
		n := takeColumnUint(t, c, 5)
		set := map[string]string{}
		var kind, name int64
		for range n {
			kind += takeSint(t, &c, 6)
			name += takeSint(t, &c, 7)
			set[resources[kind]] = resources[name]
		}
		resourceSets = append(resourceSets, set)
	}
	var result []wireMetric
	var nameID, tagID, resourceID, timestamp int64
	for len(c[10]) > 0 {
		typ := takeColumnUint(t, c, 10)
		nameID += takeSint(t, &c, 11)
		tagID += takeSint(t, &c, 12)
		resourceID += takeSint(t, &c, 13)
		m := wireMetric{Name: names[nameID], Tags: slices.Clone(tagsets[tagID]), Host: resourceSets[resourceID]["host"], Device: resourceSets[resourceID]["device"], Type: map[uint64]string{1: "count", 2: "rate", 3: "gauge"}[typ&15], Interval: int64(takeColumnUint(t, c, 14))}
		for n := takeColumnUint(t, c, 15); n > 0; n-- {
			timestamp += takeSint(t, &c, 16)
			var value float64
			switch typ & 0xf0 {
			case 0x10:
				value = float64(takeSint(t, &c, 17))
			case 0x20:
				if len(c[18]) < 4 {
					t.Fatal("short float32")
				}
				value = float64(math.Float32frombits(binary.LittleEndian.Uint32(c[18])))
				c[18] = c[18][4:]
			case 0x30:
				if len(c[19]) < 8 {
					t.Fatal("short float64")
				}
				value = math.Float64frombits(binary.LittleEndian.Uint64(c[19]))
				c[19] = c[19][8:]
			}
			m.Points = append(m.Points, [2]float64{float64(timestamp), value})
		}
		result = append(result, m)
	}
	return result
}

func first(values [][]byte) []byte {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}
func fieldNumber(values [][]byte) uint64 { value, _ := binary.Uvarint(first(values)); return value }
func takeUvarint(t *testing.T, data *[]byte) uint64 {
	t.Helper()
	value, n := binary.Uvarint(*data)
	if n <= 0 {
		t.Fatal("invalid wire varint")
	}
	*data = (*data)[n:]
	return value
}
func takeColumnUint(t *testing.T, c map[int][]byte, id int) uint64 {
	data := c[id]
	value := takeUvarint(t, &data)
	c[id] = data
	return value
}
func takeSint(t *testing.T, c *map[int][]byte, id int) int64 {
	v := takeColumnUint(t, *c, id)
	return int64(v>>1) ^ -int64(v&1)
}
func protoFields(t *testing.T, data []byte) map[int][][]byte {
	t.Helper()
	result := map[int][][]byte{}
	for len(data) > 0 {
		key := takeUvarint(t, &data)
		id, kind := int(key>>3), key&7
		var value []byte
		switch kind {
		case 0:
			before := data
			takeUvarint(t, &data)
			value = before[:len(before)-len(data)]
		case 1:
			if len(data) < 8 {
				t.Fatal("short protobuf fixed64")
			}
			value, data = data[:8], data[8:]
		case 2:
			n := takeUvarint(t, &data)
			if n > uint64(len(data)) {
				t.Fatal("short protobuf bytes")
			}
			value, data = data[:n], data[n:]
		default:
			t.Fatal("unsupported protobuf field")
		}
		result[id] = append(result[id], value)
	}
	return result
}

func TestEvidenceWaitsForSlowMetricFamiliesAndCapturesMacOSConnections(t *testing.T) {
	f := newEvidenceFixture(t, "macos", true, true, true)
	ctx := context.Background()
	for _, offset := range []time.Duration{time.Second, 16 * time.Second} {
		for _, stream := range []tc.Stream{tc.Metrics, tc.Processes, tc.Connections} {
			f.accept(ctx, t, f.record(t, stream, offset, "v2"))
		}
	}
	f.accept(ctx, t, f.record(t, tc.Metadata, time.Second, "metadata-v1"))
	for _, stream := range []tc.Stream{tc.AgentInventory, tc.HostInventory} {
		f.accept(ctx, t, f.record(t, stream, time.Second, "inventory-v1"))
	}
	f.accept(ctx, t, f.record(t, tc.Software, time.Second, ""))
	if ok, detail := f.e.Coverage(); ok || !strings.Contains(detail, "metrics/battery: 0/2") || !strings.Contains(detail, "5m0s") {
		t.Fatalf("slow scheduled family did not prevent false completion: %v %s", ok, detail)
	}
	if _, detail := f.e.Coverage(); !strings.Contains(detail, "host_system_info: 0/1") {
		t.Fatal("selected hardware stream not required for completeness")
	}
	f.accept(ctx, t, f.record(t, tc.HostSystemInfo, 3*time.Second, "inventory-v1"))
	for i, offset := range []time.Duration{2 * time.Minute, 7 * time.Minute} {
		r := f.record(t, tc.Metrics, offset, "v2")
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
