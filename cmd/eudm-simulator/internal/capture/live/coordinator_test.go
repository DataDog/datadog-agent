// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	processapi "github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

var testOptions = coordinatorOptions{requestTimeout: 200 * time.Millisecond, readinessPoll: time.Millisecond, recordPoll: time.Millisecond, heartbeat: 2 * time.Millisecond, cleanupTimeout: 200 * time.Millisecond}

type managerClient struct {
	manager         *tc.Manager
	role            string
	selected        []tc.Stream
	capCalls        atomic.Int32
	prepareCalls    atomic.Int32
	heartbeats      atomic.Int32
	stops           atomic.Int32
	reads           atomic.Int32
	capHook         func(tc.Status, int32) (tc.Status, error)
	lostPrepare     bool
	lostActivate    bool
	lostStop        bool
	lostFinalStop   bool
	disconnected    bool
	failedHeartbeat bool
	activationSkew  time.Duration
	noRecords       bool
	staleRecord     bool
	lateRecord      bool
	alterFirstRead  func(*Batch)
	panicOperation  string
}

func (c *managerClient) Role() string { return c.role }
func (c *managerClient) Capabilities(context.Context) (tc.Status, error) {
	status := c.manager.Status()
	if c.panicOperation == "discovery" {
		panic("private panic sentinel")
	}
	n := c.capCalls.Add(1)
	if c.capHook != nil {
		return c.capHook(status, n)
	}
	return status, nil
}
func (c *managerClient) Prepare(_ context.Context, request tc.PrepareRequest) (tc.Status, error) {
	c.prepareCalls.Add(1)
	c.selected = slices.Clone(request.Streams)
	status, err := c.manager.Prepare(request)
	if c.panicOperation == "prepare" {
		panic("private panic sentinel")
	}
	if err == nil && c.lostPrepare {
		return tc.Status{}, ErrUnavailable
	}
	return status, err
}
func (c *managerClient) Activate(_ context.Context, control tc.Control) (tc.Status, error) {
	status, err := c.manager.Activate(control)
	if err == nil && !c.noRecords {
		if c.staleRecord {
			c.emit(c.selected[0], status.ActivatedAt.Add(-time.Second))
		}
		for _, stream := range c.selected {
			count := 1
			if stream == tc.Metrics || stream == tc.Processes || stream == tc.Connections {
				count = 2
			}
			for range count {
				c.emit(stream, time.Now())
			}
		}
	}
	status.ActivatedAt = status.ActivatedAt.Add(c.activationSkew)
	if err == nil && c.lostActivate {
		return tc.Status{}, ErrUnavailable
	}
	return status, err
}
func (c *managerClient) Heartbeat(_ context.Context, control tc.Control) (tc.Status, error) {
	c.heartbeats.Add(1)
	if c.panicOperation == "heartbeat" {
		panic("private panic sentinel")
	}
	if c.failedHeartbeat {
		return tc.Status{}, ErrUnavailable
	}
	return c.manager.Heartbeat(control)
}
func (c *managerClient) Records(_ context.Context, request tc.ReadRequest) (Batch, error) {
	n := c.reads.Add(1)
	if c.panicOperation == "records-always" || (c.panicOperation == "records" && n == 1) {
		panic("private panic sentinel")
	}
	if c.disconnected && n == 1 {
		return Batch{}, ErrUnavailable
	}
	batch, err := c.manager.Read(request)
	if err != nil {
		return Batch{}, err
	}
	defer batch.Release()
	response := Batch{Status: batch.Status}
	for _, record := range batch.Records {
		response.Records = append(response.Records, *record)
	}
	if n == 1 && c.alterFirstRead != nil {
		c.alterFirstRead(&response)
	}
	return response, nil
}
func (c *managerClient) Stop(ctx context.Context, control tc.Control) (tc.Status, error) {
	n := c.stops.Add(1)
	if n == 1 && c.lateRecord {
		reservation := c.manager.Begin(c.selected[0], time.Now(), time.Second, 4096)
		if reservation != nil {
			go func() {
				time.Sleep(5 * time.Millisecond)
				reservation.Commit(testPayload(c.selected[0]))
			}()
		}
	}
	status, err := c.manager.Stop(ctx, control)
	if c.panicOperation == "stop" {
		panic("private panic sentinel")
	}
	if err == nil && (c.lostStop || (c.lostFinalStop && status.State == tc.Stopped)) {
		return tc.Status{}, ErrUnavailable
	}
	return status, err
}
func (c *managerClient) emit(stream tc.Stream, at time.Time) {
	reservation := c.manager.Begin(stream, at, time.Second, 4096)
	if reservation == nil {
		panic("test producer did not admit a record")
	}
	if !reservation.Commit(testPayload(stream)) {
		panic("test producer did not commit a record")
	}
}
func testPayload(stream tc.Stream) tc.Payload {
	switch stream {
	case tc.Metrics:
		return tc.Payload{Series: []tc.Series{{Name: "system.cpu.user", Ordinal: 1, Host: "test-host", Points: []tc.Point{{Timestamp: 1, Value: 2}}}}}
	case tc.Metadata:
		return tc.Payload{Metadata: &tc.HostMetadata{Hostname: "test-host", AgentVersion: "7.85.0"}}
	case tc.AgentInventory:
		return tc.Payload{Inventory: &tc.Inventory{Hostname: "test-host", UUID: "test-uuid", Timestamp: time.Now().UnixNano(), Agent: &tc.AgentInventoryMetadata{AgentVersion: "7.85.0", Flavor: "agent", InfrastructureMode: "end_user_device"}}}
	case tc.HostInventory:
		return tc.Payload{Inventory: &tc.Inventory{Hostname: "test-host", UUID: "test-uuid", Timestamp: time.Now().UnixNano(), Host: &tc.HostInventoryMetadata{AgentVersion: "7.85.0", OS: "Darwin", MemoryTotalKb: 8 << 20}}}
	case tc.Software:
		return tc.Payload{Software: &tc.Message{Body: []byte("test snapshot"), Timestamp: time.Now().UnixNano()}}
	default:
		return tc.Payload{Chunks: []tc.Chunk{{Body: []byte("test complete group")}}}
	}
}

type countingSink struct {
	platform    string
	session     Session
	counts      map[tc.Stream]int
	accepted    []tc.Record
	final       []tc.Status
	finished    bool
	failStart   bool
	failAccept  bool
	panicAccept bool
	acceptDelay time.Duration
}

func (s *countingSink) Start(_ context.Context, session Session) error {
	if s.failStart {
		return errors.New("test writer unavailable")
	}
	s.session, s.counts = session, map[tc.Stream]int{}
	return nil
}
func (s *countingSink) Accept(ctx context.Context, record tc.Record) error {
	if s.panicAccept {
		panic("private panic sentinel")
	}
	if s.failAccept {
		return errors.New("test sanitizer failed")
	}
	if s.acceptDelay > 0 {
		if err := pause(ctx, s.acceptDelay); err != nil {
			return err
		}
	}
	s.counts[record.Stream]++
	s.accepted = append(s.accepted, record)
	return nil
}
func (s *countingSink) Coverage() (bool, string) {
	complete := s.counts[tc.Metrics] >= 2 && s.counts[tc.Processes] >= 2 && s.counts[tc.Metadata] >= 1 && s.counts[tc.AgentInventory] >= 1 && s.counts[tc.HostInventory] >= 1 && s.counts[tc.Software] >= 1
	if s.platform == "windows" {
		complete = complete && s.counts[tc.Connections] >= 2
	}
	return complete, "required streams incomplete"
}
func (s *countingSink) Finish(_ context.Context, statuses []tc.Status, _ time.Time) error {
	s.final, s.finished = statuses, true
	return nil
}

func newCoordinatorFixture(t *testing.T, platform string) ([]Client, []*managerClient, *countingSink) {
	t.Helper()
	roles := []string{"core-agent", "process-agent"}
	if platform == "windows" {
		roles = append(roles, "system-probe")
	}
	var clients []Client
	var producers []*managerClient
	for _, role := range roles {
		manager := tc.NewManager(role, "7.85.0", strings.Repeat("b", 40))
		t.Cleanup(manager.Close)
		streams := []tc.Stream{tc.Metrics, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.Software}
		if role == "process-agent" {
			streams = []tc.Stream{tc.Processes}
		}
		if role == "system-probe" {
			streams = []tc.Stream{tc.Connections}
		}
		for _, stream := range streams {
			capability := tc.Capability{Stream: stream, Cadence: 15 * time.Second}
			if stream == tc.Metrics {
				capability.MetricSchedules = []tc.MetricSchedule{{Family: "cpu", Cadence: 15 * time.Second}}
			}
			if stream == tc.Connections {
				capability.ConnectionOwner = "direct"
			}
			if err := manager.Register(capability); err != nil {
				t.Fatal(err)
			}
		}
		client := &managerClient{role: role, manager: manager}
		clients, producers = append(clients, client), append(producers, client)
	}
	return clients, producers, &countingSink{platform: platform}
}

func runTestCapture(t *testing.T, platform string, clients []Client, sink Sink) error {
	t.Helper()
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	return run(ctx, platform, clients, sink, testOptions)
}

func assertDisarmed(t *testing.T, producers []*managerClient) {
	t.Helper()
	for _, p := range producers {
		if p.manager.Enabled() || (p.prepareCalls.Load() > 0 && p.stops.Load() == 0) {
			t.Fatal("capture failure left an attempted producer armed or unstopped")
		}
	}
}

func TestCoordinatorCompleteDrainsAllAcceptedSequences(t *testing.T) {
	for _, platform := range []string{"macos", "windows"} {
		t.Run(platform, func(t *testing.T) {
			clients, producers, sink := newCoordinatorFixture(t, platform)
			producers[0].staleRecord, producers[1].lateRecord = true, true
			if err := runTestCapture(t, platform, clients, sink); err != nil {
				t.Fatal(err)
			}
			if !sink.finished || len(sink.final) != len(producers) {
				t.Fatal("missing finalized producers")
			}
			if sink.counts[tc.Metrics] != 2 || sink.counts[tc.Processes] != 3 {
				t.Fatal("stale or draining records counted incorrectly")
			}
			for _, p := range producers {
				status := p.manager.Status()
				if status.State != tc.Stopped || status.FinalSequence != status.Acknowledged {
					t.Fatal("producer was not fully drained")
				}
			}
			assertDisarmed(t, producers)
		})
	}
}

func TestCoordinatorRequiresInventoryReadiness(t *testing.T) {
	for _, missing := range []tc.Stream{tc.AgentInventory, tc.HostInventory} {
		t.Run(string(missing), func(t *testing.T) {
			clients, producers, sink := newCoordinatorFixture(t, "macos")
			producers[0].manager.Unregister(missing)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			err := run(ctx, "macos", clients, sink, testOptions)
			if err == nil || !strings.Contains(err.Error(), string(missing)+"=unavailable") {
				t.Fatal("capture accepted a producer without required inventory readiness")
			}
			for _, producer := range producers {
				if producer.prepareCalls.Load() != 0 || producer.manager.Enabled() {
					t.Fatal("incomplete inventory readiness armed capture")
				}
			}
		})
	}
}

func TestCoordinatorValidatesPreactivationGroupsBeforeDiscard(t *testing.T) {
	for _, stream := range []tc.Stream{tc.Processes, tc.Connections} {
		for _, partial := range []bool{false, true} {
			t.Run(string(stream)+"/partial="+strconv.FormatBool(partial), func(t *testing.T) {
				clients, producers, sink := newCoordinatorFixture(t, "windows")
				producer := producers[1]
				if stream == tc.Connections {
					producer = producers[2]
				}
				producer.staleRecord = true
				var chunks []tc.Chunk
				for i := range 2 {
					var body model.MessageBody = &model.CollectorProc{HostName: "test-host", GroupId: 42, GroupSize: 2}
					if stream == tc.Connections {
						body = &model.CollectorConnections{HostName: "test-host", GroupId: 42, GroupSize: 2}
					}
					encoded, err := processapi.EncodePayload(body)
					if err != nil {
						t.Fatal(err)
					}
					chunks = append(chunks, tc.Chunk{Body: encoded, Headers: map[string]string{
						headers.HostHeader: "test-host", headers.RequestIDHeader: strconv.Itoa((27 << 14) + i),
					}})
				}
				if partial {
					chunks = chunks[:1]
				}
				producer.alterFirstRead = func(batch *Batch) { batch.Records[0].Payload = tc.Payload{Chunks: chunks} }
				err := runTestCapture(t, "windows", clients, sink)
				if partial {
					if err == nil || !strings.Contains(err.Error(), "incomplete process or connection group") || sink.finished {
						t.Fatalf("partial preactivation group finalized: %v", err)
					}
				} else if err != nil || !sink.finished || sink.counts[stream] != 2 {
					t.Fatalf("complete preactivation group was not discarded correctly: %v", err)
				}
				assertDisarmed(t, producers)
				status := producer.manager.Status()
				if status.State != tc.Stopped || status.Acknowledged != status.FinalSequence {
					t.Fatal("preactivation group was not acknowledged during completion or failure cleanup")
				}
			})
		}
	}
}

func TestCoordinatorWaitsForReadinessBeforePreparing(t *testing.T) {
	clients, producers, sink := newCoordinatorFixture(t, "macos")
	producers[0].capHook = func(status tc.Status, call int32) (tc.Status, error) {
		if call < 3 {
			status.Capabilities = slices.DeleteFunc(status.Capabilities, func(c tc.Capability) bool { return c.Stream == tc.Software })
		}
		return status, nil
	}
	if err := runTestCapture(t, "macos", clients, sink); err != nil {
		t.Fatal(err)
	}
	if producers[0].capCalls.Load() < 3 || producers[0].prepareCalls.Load() != 1 {
		t.Fatal("producer prepared before stream readiness")
	}
}

func TestCoordinatorSelectsExactlyOneWindowsConnectionOwner(t *testing.T) {
	for _, direct := range []bool{true, false} {
		clients, producers, sink := newCoordinatorFixture(t, "windows")
		if err := producers[1].manager.Register(tc.Capability{Stream: tc.Connections, Cadence: time.Second, ConnectionOwner: "process"}); err != nil {
			t.Fatal(err)
		}
		if !direct {
			producers[2].capHook = func(tc.Status, int32) (tc.Status, error) { return tc.Status{}, ErrIncompatible }
		}
		if err := runTestCapture(t, "windows", clients, sink); err != nil {
			t.Fatal(err)
		}
		wantRole := "process-agent"
		if direct {
			wantRole = "system-probe"
		}
		for _, record := range sink.accepted {
			if record.Stream == tc.Connections && record.Producer.Role != wantRole {
				t.Fatal("wrong connection owner")
			}
		}
		if !direct && producers[2].prepareCalls.Load() != 0 {
			t.Fatal("unselected incompatible producer was prepared")
		}
	}
}

func TestCoordinatorFailureDisarmsEveryAttemptedProducer(t *testing.T) {
	for _, kind := range []string{"lost prepare acknowledgement", "partial activation", "activation skew", "writer failure", "sanitizer failure", "sequence gap", "wrong producer", "repeated cycle", "missing stop acknowledgement", "missing final stopped acknowledgement", "reader disconnected", "heartbeat failure"} {
		t.Run(kind, func(t *testing.T) {
			clients, producers, sink := newCoordinatorFixture(t, "macos")
			switch kind {
			case "lost prepare acknowledgement":
				producers[0].lostPrepare = true
			case "partial activation":
				producers[0].lostActivate = true
			case "activation skew":
				producers[0].activationSkew = -6 * time.Second
			case "writer failure":
				sink.failStart = true
			case "sanitizer failure":
				sink.failAccept = true
			case "sequence gap":
				producers[0].alterFirstRead = func(b *Batch) { b.Records[0].Sequence++ }
			case "wrong producer":
				producers[0].alterFirstRead = func(b *Batch) { b.Records[0].Producer.InstanceID = "unrelated-producer" }
			case "repeated cycle":
				producers[0].alterFirstRead = func(b *Batch) { b.Records[1].CycleID = b.Records[0].CycleID }
			case "missing stop acknowledgement":
				producers[0].lostStop = true
			case "missing final stopped acknowledgement":
				producers[0].lostFinalStop = true
			case "reader disconnected":
				producers[0].disconnected = true
			case "heartbeat failure":
				producers[0].failedHeartbeat = true
				sink.acceptDelay = 20 * time.Millisecond
			}
			if err := runTestCapture(t, "macos", clients, sink); err == nil {
				t.Fatal("invalid capture completed")
			}
			if sink.finished {
				t.Fatal("failed capture finalized evidence")
			}
			assertDisarmed(t, producers)
			if kind == "missing stop acknowledgement" && producers[0].manager.Status().State != tc.Stopped {
				t.Fatal("cleanup did not drain a producer whose stop response was lost")
			}
		})
	}
}

func TestCoordinatorHeartbeatsContinueDuringEvidenceWork(t *testing.T) {
	clients, producers, sink := newCoordinatorFixture(t, "macos")
	sink.acceptDelay = 5 * time.Millisecond
	if err := runTestCapture(t, "macos", clients, sink); err != nil {
		t.Fatal(err)
	}
	for _, producer := range producers {
		if producer.heartbeats.Load() < 2 {
			t.Fatal("lease was not maintained while evidence was processed")
		}
	}
}

func TestCoordinatorPanicsFailClosedAndDisarmProducers(t *testing.T) {
	for _, operation := range []string{"discovery", "prepare", "records", "records-always", "heartbeat", "stop", "sink"} {
		t.Run(operation, func(t *testing.T) {
			clients, producers, sink := newCoordinatorFixture(t, "macos")
			producers[0].panicOperation = operation
			if operation == "sink" {
				sink.panicAccept = true
			}
			if operation == "heartbeat" {
				sink.acceptDelay = 20 * time.Millisecond
			}
			err := runTestCapture(t, "macos", clients, sink)
			if err == nil || strings.Contains(err.Error(), "private panic sentinel") || sink.finished {
				t.Fatal("worker panic did not fail capture privately")
			}
			assertDisarmed(t, producers)
			for _, p := range producers {
				status := p.manager.Status()
				if operation == "records-always" && p == producers[0] {
					if status.State != tc.Stopping || !strings.Contains(err.Error(), "cleanup was not fully acknowledged") {
						t.Fatal("persistent reader panic escaped bounded cleanup")
					}
					continue
				}
				if p.prepareCalls.Load() > 0 && (status.State != tc.Stopped || status.Acknowledged != status.FinalSequence) {
					t.Fatal("panic cleanup did not consume and acknowledge retained output")
				}
			}
		})
	}
}

func TestCoordinatorBoundsCycleHistory(t *testing.T) {
	_, producers, sink := newCoordinatorFixture(t, "macos")
	client := producers[0]
	control := tc.Control{ProtocolVersion: 1, SessionID: "bounded-history-session"}
	_, err := client.Prepare(context.Background(), tc.PrepareRequest{Control: control, Streams: []tc.Stream{tc.Metrics}})
	if err != nil {
		t.Fatal(err)
	}
	active, err := client.Activate(context.Background(), control)
	if err != nil {
		t.Fatal(err)
	}
	p := &producer{client: client, identity: active.Producer, streams: []tc.Stream{tc.Metrics}, activation: active, cycles: make(map[uint64]bool, maxSessionCycles)}
	for i := range maxSessionCycles {
		p.cycles[uint64(i+100)] = true
	}
	_, err = readRound(context.Background(), []*producer{p}, control, sink, false, testOptions)
	if err == nil || !strings.Contains(err.Error(), "session cycle limit") || len(p.cycles) != maxSessionCycles || p.cursor != 0 {
		t.Fatal("coordinator retained unbounded session history")
	}
}

func TestCoordinatorTimeoutReportsMissingCoverageAndCleansUp(t *testing.T) {
	clients, producers, sink := newCoordinatorFixture(t, "macos")
	producers[1].noRecords = true
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	err := run(ctx, "macos", clients, sink, testOptions)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "effective cadences") {
		t.Fatalf("missing incomplete coverage detail: %v", err)
	}
	if sink.finished {
		t.Fatal("incomplete coverage finalized")
	}
	assertDisarmed(t, producers)
	for _, producer := range producers {
		if producer.manager.Status().State != tc.Stopped {
			t.Fatal("cancelled caller context prevented independent cleanup")
		}
	}
}

func TestCoordinatorMissingStreamAndPermanentDiscoveryErrors(t *testing.T) {
	for _, kind := range []string{"missing stream", "authentication", "protocol", "transient unavailable"} {
		t.Run(kind, func(t *testing.T) {
			clients, producers, sink := newCoordinatorFixture(t, "macos")
			producers[0].capHook = func(status tc.Status, call int32) (tc.Status, error) {
				switch kind {
				case "missing stream":
					status.Capabilities = nil
				case "authentication":
					return tc.Status{}, ErrAuthentication
				case "protocol":
					status.ProtocolVersion++
				case "transient unavailable":
					if call < 3 {
						return tc.Status{}, ErrUnavailable
					}
				}
				return status, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			err := run(ctx, "macos", clients, sink, testOptions)
			if kind == "transient unavailable" {
				if err != nil || !sink.finished {
					t.Fatalf("transient readiness did not recover: %v", err)
				}
			} else if err == nil || producers[0].prepareCalls.Load() != 0 || producers[1].prepareCalls.Load() != 0 {
				t.Fatal("unready producer set was prepared")
			}
		})
	}
}

func TestCoordinatorIncludesReadyMacOSConnections(t *testing.T) {
	clients, producers, sink := newCoordinatorFixture(t, "macos")
	if err := producers[1].manager.Register(tc.Capability{Stream: tc.Connections, Cadence: 30 * time.Second, ConnectionOwner: "process"}); err != nil {
		t.Fatal(err)
	}
	if err := runTestCapture(t, "macos", clients, sink); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(producers[1].selected, tc.Connections) || sink.counts[tc.Connections] != 2 {
		t.Fatal("running macOS connection producer was omitted")
	}
	assertDisarmed(t, producers)
}

func TestCoordinatorRejectsMissingMetricSchedules(t *testing.T) {
	clients, producers, sink := newCoordinatorFixture(t, "macos")
	producers[0].capHook = func(status tc.Status, _ int32) (tc.Status, error) {
		for i := range status.Capabilities {
			status.Capabilities[i].MetricSchedules = nil
		}
		return status, nil
	}
	if err := runTestCapture(t, "macos", clients, sink); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("unproven check schedules accepted: %v", err)
	}
	assertDisarmed(t, producers)
}

func TestCoordinatorRejectsChangedMetricSchedules(t *testing.T) {
	clients, producers, sink := newCoordinatorFixture(t, "macos")
	producers[0].alterFirstRead = func(batch *Batch) {
		for i := range batch.Status.Capabilities {
			if batch.Status.Capabilities[i].Stream == tc.Metrics {
				batch.Status.Capabilities[i].MetricSchedules[0].Cadence *= 2
			}
		}
	}
	if err := runTestCapture(t, "macos", clients, sink); err == nil {
		t.Fatal("changed collection schedule was silently recorded")
	}
	assertDisarmed(t, producers)
}
