// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRCClient implements rcclient.Component for tests.
// Subscribe stores the callback in a buffered channel so tests can retrieve and
// invoke it synchronously, without spawning goroutines or relying on timers.
type mockRCClient struct {
	subscribedCh chan func(map[string]state.RawConfig, func(string, state.ApplyStatus))
}

func (m *mockRCClient) SubscribeAgentTask() {}

func (m *mockRCClient) Subscribe(_ data.Product, fn func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	m.subscribedCh <- fn
}

func newMockRCClient() *mockRCClient {
	return &mockRCClient{
		subscribedCh: make(chan func(map[string]state.RawConfig, func(string, state.ApplyStatus)), 1),
	}
}

// waitSubscribe blocks until Subscribe is called or the test times out.
func waitSubscribe(t *testing.T, rc *mockRCClient) func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
	t.Helper()
	select {
	case fn := <-rc.subscribedCh:
		return fn
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RC Subscribe call")
		return nil
	}
}

// newStreamComponent builds a minimal component wired for Stream() tests.
func newStreamComponent(t *testing.T, postgresConfigs []integration.Config) (*component, *mockRCClient) {
	t.Helper()
	rc := newMockRCClient()
	c := &component{
		log:           logmock.New(t),
		ac:            newMockAutodiscovery(t, postgresConfigs),
		rcclient:      rc,
		activeConfigs: make(map[string]integration.Config),
	}
	return c, rc
}

// buildPayloadJSON marshals a DOQueryPayload with the given parameters.
func buildPayloadJSON(t *testing.T, configID, host, dbname string, queries []QuerySpec) []byte {
	t.Helper()
	b, err := json.Marshal(DOQueryPayload{
		ConfigID:     configID,
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: host, DBName: dbname},
		Queries:      queries,
	})
	require.NoError(t, err)
	return b
}

var singleQuery = []QuerySpec{{Type: "run_query", Query: "SELECT 1", IntervalSeconds: 60, TimeoutSeconds: 10}}

// --- Stream() lifecycle tests ---

func TestStream_InitialEmptyChangesSentImmediately(t *testing.T) {
	c, _ := newStreamComponent(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh := c.Stream(ctx)

	select {
	case changes, ok := <-outCh:
		require.True(t, ok, "channel should be open")
		assert.True(t, changes.IsEmpty(), "first message should be an empty ConfigChanges to unblock LoadAndRun")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial empty ConfigChanges")
	}
}

func TestStream_ContextCancel_ClosesChannel(t *testing.T) {
	c, _ := newStreamComponent(t, nil)
	ctx, cancel := context.WithCancel(context.Background())

	outCh := c.Stream(ctx)
	<-outCh // drain initial empty

	cancel()

	select {
	case _, ok := <-outCh:
		assert.False(t, ok, "channel must be closed after context cancellation")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel to close after context cancellation")
	}
}

func TestStream_SubscribesImmediatelyWhenPostgresAvailable(t *testing.T) {
	// When postgres is already configured, Stream() must subscribe to RC without waiting
	// for the 10-second polling ticker.
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\n")},
	}
	c, rc := newStreamComponent(t, []integration.Config{postgresCfg})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Stream(ctx)

	fn := waitSubscribe(t, rc)
	assert.NotNil(t, fn)
}

func TestStream_NoPostgresAvailable_DoesNotSubscribeImmediately(t *testing.T) {
	// Without postgres, Subscribe must not be called before the first ticker tick (10s).
	c, rc := newStreamComponent(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Stream(ctx)

	select {
	case <-rc.subscribedCh:
		t.Fatal("Subscribe should not be called immediately when no postgres integration is configured")
	case <-time.After(100 * time.Millisecond):
		// Correct: no subscription within the polling window.
	}
}

// --- RC callback delivery tests ---

func TestStream_RCCallback_DeliverChangesToChannel(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\n")},
	}
	c, rc := newStreamComponent(t, []integration.Config{postgresCfg})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh := c.Stream(ctx)
	<-outCh // drain initial empty

	triggerRC := waitSubscribe(t, rc)

	payload := buildPayloadJSON(t, "cfg-1", "localhost", "mydb", singleQuery)
	triggerRC(
		map[string]state.RawConfig{"path/cfg-1": {Config: payload, Metadata: state.Metadata{ID: "rc-id-1"}}},
		func(string, state.ApplyStatus) {},
	)

	select {
	case changes := <-outCh:
		require.Len(t, changes.Schedule, 1)
		assert.Equal(t, "do_query_actions", changes.Schedule[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ConfigChanges from RC callback")
	}
}

// TestStream_ChannelReplace_PreservesUnschedule verifies that when a new RC update
// arrives while the previous update is still buffered in outCh (unread by autodiscovery),
// the dropped update's Unschedule entries are merged into the new update.
//
// Without the merge, the Unschedule is silently dropped, leaving the check permanently
// scheduled in autodiscovery (a check leak).
func TestStream_ChannelReplace_PreservesUnschedule(t *testing.T) {
	// Two separate postgres instances so each RC config can match a distinct one.
	postgresCfg := integration.Config{
		Name: "postgres",
		Instances: []integration.Data{
			integration.Data("host: localhost\ndbname: db-a\n"),
			integration.Data("host: localhost\ndbname: db-b\n"),
		},
	}
	c, rc := newStreamComponent(t, []integration.Config{postgresCfg})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh := c.Stream(ctx)
	<-outCh // drain initial empty

	triggerRC := waitSubscribe(t, rc)
	noStatus := func(string, state.ApplyStatus) {}

	// Update 1: schedule cfg-A. Drain it to simulate autodiscovery processing it —
	// cfg-A is now "in autodiscovery".
	payload1 := buildPayloadJSON(t, "cfg-A", "localhost", "db-a", singleQuery)
	triggerRC(map[string]state.RawConfig{"path/cfg-A": {Config: payload1}}, noStatus)
	update1 := <-outCh
	require.Len(t, update1.Schedule, 1, "update 1 should schedule cfg-A")

	// Update 2: remove cfg-A (empty queries = unschedule). Channel is now empty — writes directly.
	removeA, _ := json.Marshal(DOQueryPayload{ConfigID: "cfg-A"})
	triggerRC(map[string]state.RawConfig{"path/cfg-A": {Config: removeA}}, noStatus)
	// DON'T read outCh — leave update 2 ({Unschedule: [cfg-A]}) buffered.

	// Update 3: schedule cfg-B. Channel is FULL with update 2.
	// sendChanges must drain the full channel and merge update 2's Unschedule into update 3.
	payload3 := buildPayloadJSON(t, "cfg-B", "localhost", "db-b", singleQuery)
	triggerRC(map[string]state.RawConfig{"path/cfg-B": {Config: payload3}}, noStatus)

	// The merged result must contain cfg-B's Schedule AND cfg-A's Unschedule.
	// Without the merge, the Unschedule(cfg-A) would be silently dropped and cfg-A
	// would remain scheduled in autodiscovery forever.
	select {
	case changes := <-outCh:
		require.Len(t, changes.Schedule, 1, "cfg-B should be scheduled")
		assert.Equal(t, "do_query_actions", changes.Schedule[0].Name)
		require.Len(t, changes.Unschedule, 1, "cfg-A Unschedule must not be lost in the channel replace")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for merged ConfigChanges")
	}
}

// TestStream_NoPanicAfterContextCancel verifies that an RC callback firing after
// context cancellation does not panic by writing to the closed outCh.
func TestStream_NoPanicAfterContextCancel(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\n")},
	}
	c, rc := newStreamComponent(t, []integration.Config{postgresCfg})
	ctx, cancel := context.WithCancel(context.Background())

	outCh := c.Stream(ctx)
	<-outCh // drain initial empty

	triggerRC := waitSubscribe(t, rc)

	// Cancel and wait for outCh to close (signals the goroutine has exited and closed=true).
	cancel()
	for ch := range outCh { //nolint:revive
		_ = ch
	}

	// Trigger RC callback after shutdown — must not panic (write to closed channel).
	assert.NotPanics(t, func() {
		payload := buildPayloadJSON(t, "cfg-post-cancel", "localhost", "mydb", singleQuery)
		triggerRC(
			map[string]state.RawConfig{"path/cfg-post-cancel": {Config: payload}},
			func(string, state.ApplyStatus) {},
		)
	})
}
