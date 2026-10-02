// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

var testControl = Control{ProtocolVersion: ProtocolVersion, SessionID: "test-session-00000001"}

func testManager(t *testing.T, streams ...Stream) *Manager {
	t.Helper()
	m := NewManager("core-agent", "test", "fixture")
	t.Cleanup(m.Close)
	for _, stream := range streams {
		if err := m.Register(Capability{Stream: stream, Cadence: time.Minute}); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func arm(t *testing.T, m *Manager, streams ...Stream) {
	t.Helper()
	if _, err := m.Prepare(PrepareRequest{Control: testControl, Streams: streams}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Activate(testControl); err != nil {
		t.Fatal(err)
	}
}

func softwarePayload() Payload {
	return Payload{Software: &Message{Body: []byte("complete software snapshot"), Timestamp: 123456789}}
}

func observeSoftware(m *Manager) bool {
	p := softwarePayload()
	return m.Observe(Software, time.Now(), time.Minute, PayloadSize(p), func() Payload { return p })
}

func assertEmpty(t *testing.T, m *Manager) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.current; s != nil && (s.bytes != 0 || s.records != 0 || s.pending != 0 || len(s.queue) != 0 || s.reading) {
		t.Fatalf("retained capture data: bytes=%d records=%d pending=%d queued=%d reading=%v", s.bytes, s.records, s.pending, len(s.queue), s.reading)
	}
}

func TestDisabledRetainsNothing(t *testing.T) {
	m := testManager(t, Software)
	collectedAt := time.Now()
	allocs := testing.AllocsPerRun(1000, func() {
		if m.Enabled() || m.Begin(Software, collectedAt, time.Minute, 1024) != nil {
			t.Fatal("disabled capture admitted work")
		}
	})
	if allocs != 0 {
		t.Fatalf("disabled path allocated: %v", allocs)
	}
	called := false
	if m.Observe(Software, collectedAt, time.Minute, 10, func() Payload { called = true; return Payload{} }) || called {
		t.Fatal("disabled observer copied")
	}
	assertEmpty(t, m)
}

func TestMetricScheduleCapabilitiesOwnAndBoundTheirSlices(t *testing.T) {
	m := testManager(t)
	schedules := []MetricSchedule{{Family: "battery", Cadence: 5 * time.Minute}}
	if err := m.Register(Capability{Stream: Metrics, Cadence: 15 * time.Second, MetricSchedules: schedules}); err != nil {
		t.Fatal(err)
	}
	schedules[0].Cadence = time.Hour
	status := m.Status()
	if status.Capabilities[0].MetricSchedules[0].Cadence != 5*time.Minute {
		t.Fatal("manager retained caller-owned capability schedules")
	}
	status.Capabilities[0].MetricSchedules[0].Cadence = time.Hour
	if m.Status().Capabilities[0].MetricSchedules[0].Cadence != 5*time.Minute {
		t.Fatal("status exposed mutable manager schedules")
	}
	for _, invalid := range [][]MetricSchedule{
		{{Family: "private-instance", Cadence: time.Minute}},
		{{Family: "battery", Cadence: 0}},
		{{Family: "battery", Cadence: time.Minute}, {Family: "battery", Cadence: time.Minute}},
		make([]MetricSchedule, 8),
	} {
		if err := m.Register(Capability{Stream: Metrics, Cadence: time.Second, MetricSchedules: invalid}); !errors.Is(err, ErrRequest) {
			t.Fatal("invalid metric schedules registered")
		}
	}
	if err := m.Register(Capability{Stream: Software, Cadence: time.Minute, MetricSchedules: schedules}); !errors.Is(err, ErrRequest) {
		t.Fatal("non-metric producer advertised metric schedules")
	}
}

func TestLifecycleAndAcknowledgement(t *testing.T) {
	m := testManager(t, Software)
	prepared, err := m.Prepare(PrepareRequest{Control: testControl, Streams: []Stream{Software}})
	if err != nil || prepared.State != Prepared || m.Enabled() {
		t.Fatalf("prepare: %v %v", prepared.State, err)
	}
	if observeSoftware(m) {
		t.Fatal("prepared capture admitted work")
	}
	active, err := m.Activate(testControl)
	if err != nil || active.State != Active {
		t.Fatalf("activate: %v", err)
	}
	repeated, err := m.Activate(testControl)
	if err != nil || !repeated.ActivatedAt.Equal(active.ActivatedAt) {
		t.Fatal("activation is not idempotent")
	}
	for range 2 {
		if !observeSoftware(m) {
			t.Fatal("observation rejected")
		}
	}
	status, err := m.Stop(context.Background(), testControl)
	if err != nil || status.State != Stopping || status.FinalSequence != 2 || m.Enabled() {
		t.Fatalf("stop: %+v %v", status, err)
	}
	if observeSoftware(m) {
		t.Fatal("stop did not close admission")
	}
	batch, err := m.Read(ReadRequest{Control: testControl})
	if err != nil || len(batch.Records) != 2 {
		t.Fatalf("read: %v", err)
	}
	for i, record := range batch.Records {
		if record.Sequence != uint64(i+1) || record.CycleID == 0 || record.SessionID != testControl.SessionID || record.Producer != active.Producer || record.CollectedAt.IsZero() || record.ObservedAt.Before(record.CollectedAt) {
			t.Fatal("invalid record envelope")
		}
	}
	if batch.Records[0].CycleID == batch.Records[1].CycleID {
		t.Fatal("distinct observations share a cycle")
	}
	batch.Release()
	retry, err := m.Read(ReadRequest{Control: testControl})
	if err != nil || len(retry.Records) != 2 || retry.Records[1].Sequence != 2 {
		t.Fatal("retry manufactured observations")
	}
	retry.Release()
	ack, err := m.Read(ReadRequest{Control: testControl, Cursor: 2})
	if err != nil || len(ack.Records) != 0 || ack.Status.State != Stopped {
		t.Fatalf("ack: %v", err)
	}
	ack.Release()
	stopped, err := m.Stop(context.Background(), testControl)
	if err != nil || stopped.State != Stopped || stopped.Acknowledged != 2 || stopped.StoppedAt.IsZero() {
		t.Fatalf("stopped: %+v %v", stopped, err)
	}
	assertEmpty(t, m)
}

func TestCompetingAndObsoleteSessions(t *testing.T) {
	m := testManager(t, Software, Metrics)
	arm(t, m, Software)
	other := Control{ProtocolVersion: ProtocolVersion, SessionID: "test-session-00000002"}
	if _, err := m.Prepare(PrepareRequest{Control: other, Streams: []Stream{Software}}); !errors.Is(err, ErrBusy) {
		t.Fatalf("competing prepare: %v", err)
	}
	if _, err := m.Prepare(PrepareRequest{Control: testControl, Streams: []Stream{Metrics}}); !errors.Is(err, ErrRequest) {
		t.Fatalf("changed preparation: %v", err)
	}
	if _, err := m.Stop(context.Background(), testControl); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Prepare(PrepareRequest{Control: other, Streams: []Stream{Software}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Activate(other); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func(Control) (Status, error){m.Activate, m.Heartbeat, func(c Control) (Status, error) { return m.Stop(context.Background(), c) }} {
		if _, err := operation(testControl); !errors.Is(err, ErrSession) {
			t.Fatalf("stale control: %v", err)
		}
	}
	if err := m.Fail(testControl); !errors.Is(err, ErrSession) {
		t.Fatalf("stale failure: %v", err)
	}
	if _, err := m.Read(ReadRequest{Control: testControl, Cursor: 99}); !errors.Is(err, ErrSession) {
		t.Fatalf("stale read: %v", err)
	}
	if !m.Enabled() || m.Status().SessionID != other.SessionID {
		t.Fatal("obsolete session affected current session")
	}
}

func TestSelectionAndCapabilities(t *testing.T) {
	m := testManager(t, Software)
	if _, err := m.Prepare(PrepareRequest{Control: testControl, Streams: []Stream{Connections}}); !errors.Is(err, ErrState) {
		t.Fatalf("unavailable stream: %v", err)
	}
	arm(t, m, Software)
	if m.Begin(Metrics, time.Now(), time.Second, 20) != nil {
		t.Fatal("unselected stream admitted")
	}
	if !observeSoftware(m) {
		t.Fatal("selected stream rejected")
	}
	m.Unregister(Software)
	if m.Enabled() || m.Status().State != Failed || len(m.Status().Capabilities) != 0 {
		t.Fatal("producer shutdown not reflected")
	}
	assertEmpty(t, m)
}

func TestRecordCapacityIncludesPendingAndUnacknowledged(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	pending := m.Begin(Software, time.Now(), time.Minute, 512)
	for i := 0; i < MaxRecords-1; i++ {
		if !observeSoftware(m) {
			t.Fatalf("capacity exhausted at %d", i)
		}
	}
	batch, err := m.Read(ReadRequest{Control: testControl})
	if err != nil || len(batch.Records) != MaxBatchRecords {
		t.Fatalf("bounded read: %v", err)
	}
	batch.Release()
	var copied bool
	if m.Observe(Software, time.Now(), time.Minute, 512, func() Payload { copied = true; return softwarePayload() }) || copied {
		t.Fatal("full queue copied or accepted")
	}
	if status := m.Status(); status.State != Failed || status.Failures != 1 || status.Drops != 1 {
		t.Fatalf("overflow: %+v", status)
	}
	pending.Discard()
	assertEmpty(t, m)
}

func TestByteAndItemLimits(t *testing.T) {
	for _, bytes := range []int64{-1, MaxItemBytes, math.MaxInt64} {
		t.Run("oversize", func(t *testing.T) {
			m := testManager(t, Software)
			arm(t, m, Software)
			if m.Begin(Software, time.Now(), time.Minute, bytes) != nil || m.Status().State != Failed {
				t.Fatal("invalid size admitted")
			}
			assertEmpty(t, m)
		})
	}
	m := testManager(t, Software)
	arm(t, m, Software)
	a := m.Begin(Software, time.Now(), time.Minute, MaxItemBytes-RecordOverhead)
	b := m.Begin(Software, time.Now(), time.Minute, MaxItemBytes-RecordOverhead)
	if a == nil || b == nil {
		t.Fatal("exact budget rejected")
	}
	if m.Begin(Software, time.Now(), time.Minute, 0) != nil {
		t.Fatal("producer byte limit exceeded")
	}
	a.Discard()
	b.Discard()
	assertEmpty(t, m)
}

func TestPendingAssemblyGrowthAndUndercount(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	r := m.Begin(Software, time.Now(), time.Minute, 0)
	if r == nil || !r.Grow(MaxItemBytes-RecordOverhead) {
		t.Fatal("growth rejected")
	}
	if r.Grow(1) || m.Status().State != Failed {
		t.Fatal("oversize assembly accepted")
	}
	r.Discard()
	assertEmpty(t, m)
	m = testManager(t, Software)
	arm(t, m, Software)
	r = m.Begin(Software, time.Now(), time.Minute, 1)
	if r.Commit(softwarePayload()) || m.Status().State != Failed {
		t.Fatal("undercounted copy accepted")
	}
	assertEmpty(t, m)
}

func TestLeaseAndHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t, Software)
		arm(t, m, Software)
		if !observeSoftware(m) {
			t.Fatal("copy rejected")
		}
		for i := 0; i < 12; i++ {
			time.Sleep(HeartbeatInterval)
			if _, err := m.Heartbeat(testControl); err != nil || !m.Enabled() {
				t.Fatal("renewed lease expired")
			}
		}
		time.Sleep(LeaseDuration + time.Second)
		synctest.Wait()
		if m.Enabled() || m.Status().State != Failed {
			t.Fatal("expired lease still armed")
		}
		if _, err := m.Activate(testControl); err != nil || m.Enabled() {
			t.Fatal("expired session resurrected")
		}
		assertEmpty(t, m)
	})
}

func TestPreparedLeaseExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t, Software)
		if _, err := m.Prepare(PrepareRequest{Control: testControl, Streams: []Stream{Software}}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(LeaseDuration + time.Second)
		synctest.Wait()
		if m.Status().State != Failed {
			t.Fatal("preparation did not expire")
		}
		assertEmpty(t, m)
	})
}

func TestStopWaitsForCopyAndDrains(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t, Software)
		arm(t, m, Software)
		started, finish := make(chan struct{}), make(chan struct{})
		var accepted atomic.Bool
		go func() {
			accepted.Store(m.Observe(Software, time.Now(), time.Minute, 1024, func() Payload {
				close(started)
				<-finish
				return softwarePayload()
			}))
		}()
		<-started
		stopped := make(chan Status, 1)
		go func() {
			status, err := m.Stop(context.Background(), testControl)
			if err != nil {
				t.Error(err)
			}
			stopped <- status
		}()
		synctest.Wait()
		if m.Enabled() {
			t.Fatal("admission still open")
		}
		select {
		case <-stopped:
			t.Fatal("stop returned before copy completed")
		default:
		}
		close(finish)
		status := <-stopped
		synctest.Wait()
		if !accepted.Load() || status.State != Stopping || status.FinalSequence != 1 {
			t.Fatal("stop lost admitted copy")
		}
		batch, err := m.Read(ReadRequest{Control: testControl})
		if err != nil || len(batch.Records) != 1 {
			t.Fatal("accepted record not drainable")
		}
		batch.Release()
		ack, err := m.Read(ReadRequest{Control: testControl, Cursor: 1})
		if err != nil {
			t.Fatal(err)
		}
		ack.Release()
		assertEmpty(t, m)
	})
}

func TestFailureWhileCopyingAndReading(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	if !observeSoftware(m) {
		t.Fatal("copy rejected")
	}
	r := m.Begin(Software, time.Now(), time.Minute, 1024)
	batch, err := m.Read(ReadRequest{Control: testControl})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Fail(testControl); err != nil {
		t.Fatal(err)
	}
	if len(batch.Records[0].Payload.Software.Body) == 0 {
		t.Fatal("failure raced with reader")
	}
	if r.Commit(softwarePayload()) {
		t.Fatal("failed pending copy admitted")
	}
	m.mu.Lock()
	if m.current.bytes == 0 || m.current.records != 1 {
		t.Error("pinned record not charged")
	}
	m.mu.Unlock()
	batch.Release()
	batch.Release()
	assertEmpty(t, m)
}

func TestCopyPanicAndShutdownDoNotEscape(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	if m.Observe(Software, time.Now(), time.Minute, 100, func() Payload { panic("sensitive producer detail") }) {
		t.Fatal("panic accepted")
	}
	if m.Status().Failures != 1 {
		t.Fatal("panic not recorded")
	}
	assertEmpty(t, m)
	m = testManager(t, Software)
	arm(t, m, Software)
	if !observeSoftware(m) {
		t.Fatal("copy rejected")
	}
	m.Close()
	m.Close()
	if m.Enabled() || m.Status().State != Failed || len(m.Status().Capabilities) != 0 {
		t.Fatal("shutdown did not disarm")
	}
	assertEmpty(t, m)
}

func TestInvalidAndRepeatedCursors(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	if !observeSoftware(m) {
		t.Fatal("copy rejected")
	}
	if _, err := m.Read(ReadRequest{Control: testControl, Cursor: 1}); !errors.Is(err, ErrCursor) {
		t.Fatal("acknowledged unseen record")
	}
	batch, err := m.Read(ReadRequest{Control: testControl})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Read(ReadRequest{Control: testControl}); !errors.Is(err, ErrBusy) {
		t.Fatal("unbounded concurrent reader")
	}
	batch.Release()
	ack, err := m.Read(ReadRequest{Control: testControl, Cursor: 1})
	if err != nil {
		t.Fatal(err)
	}
	ack.Release()
	if _, err := m.Read(ReadRequest{Control: testControl}); !errors.Is(err, ErrCursor) {
		t.Fatal("stale cursor accepted")
	}
	assertEmpty(t, m)
}

func TestConcurrentObservationAndStop(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	var accepted atomic.Uint64
	var workers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 64; i++ {
		workers.Go(func() {
			<-start
			if observeSoftware(m) {
				accepted.Add(1)
			}
		})
	}
	close(start)
	if _, err := m.Stop(context.Background(), testControl); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	var cursor uint64
	for {
		batch, err := m.Read(ReadRequest{Control: testControl, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range batch.Records {
			if record.Sequence != cursor+1 {
				t.Fatal("sequence gap or reorder")
			}
			cursor = record.Sequence
		}
		empty := len(batch.Records) == 0
		batch.Release()
		if empty {
			break
		}
	}
	if cursor != accepted.Load() || m.Status().State != Stopped {
		t.Fatal("stop lost accepted observations")
	}
	assertEmpty(t, m)
}

func TestEffectiveCadenceCanAdvanceDuringActiveCapture(t *testing.T) {
	m := NewManager("core-agent", "fixture", "fixture")
	defer m.Close()
	if err := m.Register(Capability{Stream: Metadata, Cadence: 5 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	c := Control{ProtocolVersion: ProtocolVersion, SessionID: "metadata-backoff-session"}
	if _, err := m.Prepare(PrepareRequest{Control: c, Streams: []Stream{Metadata}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Activate(c); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(Capability{Stream: Metadata, Cadence: 15 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	status := m.Status()
	if status.State != Active || status.Capabilities[0].Cadence != 15*time.Minute {
		t.Fatal("normal backoff disarmed capture or left stale readiness")
	}
}
