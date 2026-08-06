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
	"gopkg.in/yaml.v3"
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
		activeConfigs: make(map[string]activeConfigEntry),
		managedBases:  make(map[string]*managedBaseEntry),
	}
	return c, rc
}

// buildPayloadJSON marshals a DOQueryPayload with the given parameters.
func buildPayloadJSON(t *testing.T, configID, host string, queries []QuerySpec) []byte {
	t.Helper()
	b, err := json.Marshal(DOQueryPayload{
		ConfigID:     configID,
		DBIdentifier: DBIdentifier{Type: "self-hosted", Host: host},
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
	// When postgres is already configured with data_observability.enabled, Stream() must
	// subscribe to RC without waiting for the 10-second polling ticker.
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\ndata_observability:\n  enabled: true\n")},
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
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\ndata_observability:\n  enabled: true\n")},
	}
	c, rc := newStreamComponent(t, []integration.Config{postgresCfg})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh := c.Stream(ctx)
	<-outCh // drain initial empty

	triggerRC := waitSubscribe(t, rc)

	payload := buildPayloadJSON(t, "cfg-1", "localhost", singleQuery)
	triggerRC(
		map[string]state.RawConfig{"path/cfg-1": {Config: payload, Metadata: state.Metadata{ID: "rc-id-1"}}},
		func(string, state.ApplyStatus) {},
	)

	select {
	case changes := <-outCh:
		require.Len(t, changes.Schedule, 1)
		assert.Equal(t, "postgres", changes.Schedule[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ConfigChanges from RC callback")
	}
}

// TestStream_ChannelReplace_PreservesUnschedule covers a capacity-one channel replacement:
// update 2 is still waiting for autodiscovery when update 3 arrives, so update 2 must be
// removed from the channel to make room. Its Schedule entries are stale and must be dropped,
// but its Unschedule entries must be carried into update 3; otherwise configs already
// processed from update 1 would remain running in autodiscovery.
func TestStream_ChannelReplace_PreservesUnschedule(t *testing.T) {
	// Use two instances because applying one query action splits the base config into two
	// scheduled configs: the matched instance with its query and the unmatched remainder.
	postgresCfg := integration.Config{
		Name: "postgres",
		Instances: []integration.Data{
			integration.Data("host: db-a.internal\ndbname: db-a\ndata_observability:\n  enabled: true\n"),
			integration.Data("host: db-b.internal\ndbname: db-b\ndata_observability:\n  enabled: true\n"),
		},
	}
	c, rc := newStreamComponent(t, []integration.Config{postgresCfg})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh := c.Stream(ctx)
	<-outCh // drain initial empty

	triggerRC := waitSubscribe(t, rc)
	noStatus := func(string, state.ApplyStatus) {}

	// Update 1 targets db-a. Consume the update to simulate autodiscovery applying both
	// scheduled configs: cfg-A for db-a and the unchanged db-b remainder. We save their
	// digests because both must later be removed, not merely an arbitrary pair of configs.
	payload1 := buildPayloadJSON(t, "cfg-A", "db-a.internal", singleQuery)
	triggerRC(map[string]state.RawConfig{"path/cfg-A": {Config: payload1}}, noStatus)
	update1 := <-outCh
	require.Len(t, update1.Schedule, 2, "update 1 should schedule cfg-A and the db-b remainder")
	update1ScheduleDigests := make([]string, 0, len(update1.Schedule))
	for _, cfg := range update1.Schedule {
		update1ScheduleDigests = append(update1ScheduleDigests, cfg.Digest())
	}

	// Update 2 removes cfg-A. It therefore unschedules both configs applied in update 1 and
	// schedules the original two-instance base config as their replacement. Leave this update
	// unread so it occupies the channel when update 3 arrives.
	removeA, _ := json.Marshal(DOQueryPayload{ConfigID: "cfg-A"})
	triggerRC(map[string]state.RawConfig{"path/cfg-A": {Config: removeA}}, noStatus)
	// Do not read outCh: update 2 must remain buffered for this regression scenario.

	// Update 3 targets db-b while update 2 is buffered. The latest snapshot schedules cfg-B
	// for db-b plus the db-a remainder, and unschedules the original base config. sendChanges
	// must also carry forward update 2's two Unschedules, but must discard update 2's now-stale
	// base restoration.
	payload3 := buildPayloadJSON(t, "cfg-B", "db-b.internal", singleQuery)
	triggerRC(map[string]state.RawConfig{"path/cfg-B": {Config: payload3}}, noStatus)

	select {
	case changes := <-outCh:
		require.Len(t, changes.Schedule, 2, "should contain cfg-B and its remainder (dropped base restoration is discarded)")

		// Assert the identities and roles of the two latest configs, not only their count. This
		// catches the regression where update 2's stale base restoration survives replacement.
		scheduledHosts := make(map[string]bool, len(changes.Schedule))
		for _, cfg := range changes.Schedule {
			assert.NotEqual(t, postgresCfg.Digest(), cfg.Digest(), "dropped base restoration must not be scheduled")
			require.Len(t, cfg.Instances, 1)
			var instance map[string]any
			require.NoError(t, yaml.Unmarshal(cfg.Instances[0], &instance))
			host, _ := instance["host"].(string)
			scheduledHosts[host] = true
			doConfig, _ := instance["data_observability"].(map[string]any)
			_, hasQueries := doConfig["queries"]
			switch host {
			case "db-a.internal":
				assert.False(t, hasQueries, "db-a should be the plain remainder instance")
			case "db-b.internal":
				assert.True(t, hasQueries, "db-b should be the latest DO check")
			default:
				t.Errorf("unexpected scheduled host %q", host)
			}
		}
		assert.Equal(t, map[string]bool{"db-a.internal": true, "db-b.internal": true}, scheduledHosts)

		// The final update must remove exactly three configs: cfg-A and the db-b remainder from
		// update 1, carried forward from dropped update 2, plus the original base removed by
		// update 3. Comparing digests ensures no stale update-1 config remains active.
		expectedUnscheduleDigests := append(append([]string(nil), update1ScheduleDigests...), postgresCfg.Digest())
		actualUnscheduleDigests := make([]string, 0, len(changes.Unschedule))
		for _, cfg := range changes.Unschedule {
			actualUnscheduleDigests = append(actualUnscheduleDigests, cfg.Digest())
		}
		assert.ElementsMatch(t, expectedUnscheduleDigests, actualUnscheduleDigests)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for merged ConfigChanges")
	}
}

// TestStream_NoPanicAfterContextCancel verifies that an RC callback firing after
// context cancellation does not panic by writing to the closed outCh.
func TestStream_NoPanicAfterContextCancel(t *testing.T) {
	postgresCfg := integration.Config{
		Name:      "postgres",
		Instances: []integration.Data{integration.Data("host: localhost\ndbname: mydb\ndata_observability:\n  enabled: true\n")},
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
		payload := buildPayloadJSON(t, "cfg-post-cancel", "localhost", singleQuery)
		triggerRC(
			map[string]state.RawConfig{"path/cfg-post-cancel": {Config: payload}},
			func(string, state.ApplyStatus) {},
		)
	})
}
