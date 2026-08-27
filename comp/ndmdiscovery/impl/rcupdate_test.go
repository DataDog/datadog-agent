// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type applyRecorder struct {
	mu       sync.Mutex
	statuses map[string]state.ApplyStatus
}

func newApplyRecorder() *applyRecorder {
	return &applyRecorder{statuses: map[string]state.ApplyStatus{}}
}

func (a *applyRecorder) callback(path string, st state.ApplyStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statuses[path] = st
}

func (a *applyRecorder) get(path string) (state.ApplyStatus, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.statuses[path]
	return st, ok
}

func (a *applyRecorder) len() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.statuses)
}

func rawConfig(body string) state.RawConfig {
	return state.RawConfig{Config: []byte(body)}
}

func autodiscoveryBody(id, cidr string) string {
	return `{"kind":"autodiscovery","autodiscovery_id":"` + id + `","cidr":"` + cidr + `","credential_ids":["cred-a"],"interval_sec":3600}`
}

func newTestRCHandler(t *testing.T) (*rcHandler, *scheduler) {
	t.Helper()
	sched, _ := newTestScheduler(t, answerAll(), 10)
	sched.start(context.Background())
	t.Cleanup(sched.stop)

	h := newRCHandler(sched, logmock.New(t), rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536})
	return h, sched
}

func TestRCUpdateSchedulesRange(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"datadog/2/NDM/ad-1/config": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)

	assert.Equal(t, 1, sched.count())
	st, ok := rec.get("datadog/2/NDM/ad-1/config")
	require.True(t, ok)
	assert.Equal(t, state.ApplyStateAcknowledged, st.State)
	assert.Empty(t, st.Error)
}

func TestRCUpdateIgnoresForeignKindsSilently(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"datadog/2/NDM/other-1/config": rawConfig(`{"kind":"monitored_devices","devices":[]}`),
	}, rec.callback)

	assert.Equal(t, 0, sched.count())
	assert.Equal(t, 0, rec.len(),
		"the NDM product is shared, so another subscriber acknowledges its own configs")
}

func TestRCUpdateIgnoresKindlessConfigsSilently(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	// A payload with no kind at all is not this component's to claim either.
	h.Update(map[string]state.RawConfig{
		"datadog/2/NDM/other-1/config": rawConfig(`{"devices":[]}`),
	}, rec.callback)

	assert.Equal(t, 0, sched.count())
	assert.Equal(t, 0, rec.len())
}

func TestRCUpdateHandlesMixedKinds(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"datadog/2/NDM/ad-1/config":    rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
		"datadog/2/NDM/other-1/config": rawConfig(`{"kind":"monitored_devices"}`),
	}, rec.callback)

	assert.Equal(t, 1, sched.count())
	assert.Equal(t, 1, rec.len())
	_, ok := rec.get("datadog/2/NDM/ad-1/config")
	assert.True(t, ok)
}

func TestRCUpdateReportsParseErrors(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(`{"kind":"autodiscovery","cidr":"10.0.0.0/24","credential_ids":["cred-a"]}`),
		"p2": rawConfig(`{"kind":"autodiscovery","autodiscovery_id":"ad-2","cidr":"nope","credential_ids":["cred-a"]}`),
	}, rec.callback)

	assert.Equal(t, 0, sched.count())

	st, ok := rec.get("p1")
	require.True(t, ok)
	assert.Equal(t, state.ApplyStateError, st.State)
	assert.Contains(t, st.Error, "autodiscovery_id")

	st, ok = rec.get("p2")
	require.True(t, ok)
	assert.Equal(t, state.ApplyStateError, st.State)
	assert.Contains(t, st.Error, "invalid CIDR")
}

func TestRCUpdateAppliesGoodConfigsAlongsideBadOnes(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"bad":  rawConfig(`{"kind":"autodiscovery","cidr":"10.0.0.0/24"}`),
		"good": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)

	assert.Equal(t, 1, sched.count(), "one bad config does not stop the rest of the snapshot from applying")
	st, ok := rec.get("good")
	require.True(t, ok)
	assert.Equal(t, state.ApplyStateAcknowledged, st.State)
	st, ok = rec.get("bad")
	require.True(t, ok)
	assert.Equal(t, state.ApplyStateError, st.State)
}

func TestRCUpdateReportsSchedulerRejection(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	body := `{"kind":"autodiscovery","autodiscovery_id":"ad-1","cidr":"10.0.0.0/24","credential_ids":["missing-cred"]}`
	h.Update(map[string]state.RawConfig{"p1": rawConfig(body)}, rec.callback)

	assert.Equal(t, 0, sched.count())
	st, ok := rec.get("p1")
	require.True(t, ok)
	assert.Equal(t, state.ApplyStateError, st.State)
	assert.Contains(t, st.Error, "missing-cred")
}

func TestRCUpdateDeletesAbsentRanges(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
		"p2": rawConfig(autodiscoveryBody("ad-2", "10.0.1.0/24")),
	}, rec.callback)
	require.Equal(t, 2, sched.count())

	// A full snapshot with p2 gone means ad-2 was deleted.
	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	assert.Equal(t, 1, sched.count())

	// An empty snapshot removes everything.
	h.Update(map[string]state.RawConfig{}, rec.callback)
	assert.Equal(t, 0, sched.count())
}

func TestRCUpdateDropsNewlyInvalidRangeFromActiveSet(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	require.Equal(t, 1, sched.count())

	// The same path now holds an unusable config.
	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(`{"kind":"autodiscovery","autodiscovery_id":"ad-1","cidr":"10.0.0.0/12","credential_ids":["cred-a"]}`),
	}, rec.callback)

	assert.Equal(t, 0, sched.count(), "an unusable config stops the range it replaces")
	st, _ := rec.get("p1")
	assert.Equal(t, state.ApplyStateError, st.State)

	// The range left the active set, so a snapshot re-adding it is treated as new.
	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	assert.Equal(t, 1, sched.count())
	st, _ = rec.get("p1")
	assert.Equal(t, state.ApplyStateAcknowledged, st.State)
}

func TestRCUpdateIgnoresMalformedJSONSilently(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	// The kind cannot be read, so this config cannot be claimed.
	h.Update(map[string]state.RawConfig{"p1": rawConfig(`not json`)}, rec.callback)

	assert.Equal(t, 0, sched.count())
	assert.Equal(t, 0, rec.len())
}

// scheduledIDs reports which autodiscovery IDs the scheduler currently runs,
// so a test can tell "the right number of ranges" from "the right ranges".
func scheduledIDs(s *scheduler) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.ranges))
	for id := range s.ranges {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func TestRCUpdateStopsPreviousRangeWhenAutodiscoveryIDChanges(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	require.Equal(t, []string{"ad-1"}, scheduledIDs(sched))

	// The same RC path now names a different range. A fresh recorder, so the
	// acknowledgement asserted below is this snapshot's and not the first one's.
	second := newApplyRecorder()
	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-2", "10.0.1.0/24")),
	}, second.callback)

	assert.Equal(t, []string{"ad-2"}, scheduledIDs(sched),
		"the range a path used to name stops when that path's autodiscovery ID changes")
	st, ok := second.get("p1")
	require.True(t, ok)
	assert.Equal(t, state.ApplyStateAcknowledged, st.State)
}

func TestRCUpdateDropsClaimedRangeThatBecomesMalformed(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	require.Equal(t, 1, sched.count())

	after := newApplyRecorder()
	h.Update(map[string]state.RawConfig{"p1": rawConfig(`not json`)}, after.callback)

	assert.Equal(t, 0, sched.count(), "a claimed range stops sweeping once its payload becomes unreadable")
	assert.Equal(t, 0, after.len(),
		"an unreadable payload is still not acknowledged: this component cannot claim it")
}

func TestRCUpdateDropsClaimedRangeThatBecomesForeignKind(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	require.Equal(t, 1, sched.count())

	after := newApplyRecorder()
	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(`{"kind":"monitored_devices","devices":[]}`),
	}, after.callback)

	assert.Equal(t, 0, sched.count(),
		"a claimed range stops sweeping once its path turns into someone else's config")
	assert.Equal(t, 0, after.len(),
		"a foreign kind is still not acknowledged: another subscriber owns it")
}

func TestRCUpdateKeepsRangeNamedByASecondPath(t *testing.T) {
	h, sched := newTestRCHandler(t)
	rec := newApplyRecorder()

	h.Update(map[string]state.RawConfig{
		"p1": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
		"p2": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	require.Equal(t, []string{"ad-1"}, scheduledIDs(sched))

	// p1 is gone from the snapshot, but p2 still names ad-1.
	h.Update(map[string]state.RawConfig{
		"p2": rawConfig(autodiscoveryBody("ad-1", "10.0.0.0/24")),
	}, rec.callback)
	assert.Equal(t, []string{"ad-1"}, scheduledIDs(sched),
		"another path still names this range, so dropping one path must not stop it")

	// With the last path gone, the range stops.
	h.Update(map[string]state.RawConfig{}, rec.callback)
	assert.Empty(t, scheduledIDs(sched))
}
