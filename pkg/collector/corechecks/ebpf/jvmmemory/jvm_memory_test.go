// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package jvmmemory

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/jvmmemory/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// fakeSystemProbe is an http.RoundTripper serving the system-probe endpoints
// used by the check.
type fakeSystemProbe struct {
	mu       sync.Mutex
	response []model.JVMStats
	status   int
}

func (f *fakeSystemProbe) set(status int, response []model.JVMStats) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
	f.response = response
}

func (f *fakeSystemProbe) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	rec := httptest.NewRecorder()
	switch req.URL.Path {
	case "/debug/stats": // startup check of the system-probe client
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.WriteString("{}")
	case "/jvm_memory/check":
		if f.status != http.StatusOK {
			rec.WriteHeader(f.status)
			break
		}
		body, err := json.Marshal(f.response)
		if err != nil {
			return nil, err
		}
		rec.Header().Set("Content-Type", "application/json")
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.Write(body)
	default:
		rec.WriteHeader(http.StatusNotFound)
	}
	return rec.Result(), nil
}

type testCheck struct {
	check     *Check
	sender    *mocksender.MockSender
	sysProbe  *fakeSystemProbe
	tagger    taggermock.Mock
	clockTime time.Time
}

func newTestCheck(t *testing.T, instanceConfig string) *testCheck {
	tc := &testCheck{
		tagger:    taggerfxmock.SetupFakeTagger(t),
		sysProbe:  &fakeSystemProbe{status: http.StatusOK},
		clockTime: time.Unix(1_800_000_000, 0),
	}
	tc.check = newCheck(tc.tagger)
	tc.sender = mocksender.NewMockSender(t, tc.check.ID())
	tc.sender.SetupAcceptAll()
	require.NoError(t, tc.check.Configure(tc.sender.GetSenderManager(), integration.FakeConfigHash, integration.Data(instanceConfig), nil, "test", "test"))

	tc.check.sysProbeClient = sysprobeclient.NewCheckClient(
		&http.Client{Transport: tc.sysProbe},
		&http.Client{Transport: tc.sysProbe},
	)
	tc.check.now = func() time.Time { return tc.clockTime }
	return tc
}

// at returns the wall clock time, in Unix nanoseconds, at the given offset
// from the current time of the fake clock.
func (tc *testCheck) at(offset time.Duration) int64 {
	return tc.clockTime.Add(offset).UnixNano()
}

func (tc *testCheck) advance(d time.Duration) {
	tc.clockTime = tc.clockTime.Add(d)
}

type metricKey struct {
	method string
	name   string
	tags   string
}

func seriesKey(method, name string, tags ...string) metricKey {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)
	return metricKey{method: method, name: name, tags: strings.Join(sorted, ",")}
}

// run runs the check and returns the submitted metrics. It fails the test if
// the same series is submitted twice in a run, or if Commit is not called.
func (tc *testCheck) run(t *testing.T) map[metricKey]float64 {
	t.Helper()
	tc.sender.ResetCalls()
	require.NoError(t, tc.check.Run())

	metrics := make(map[metricKey]float64)
	committed := false
	for _, call := range tc.sender.Calls {
		switch call.Method {
		case "Gauge", "Count":
			assert.Empty(t, call.Arguments.String(2), "unexpected hostname")
			k := seriesKey(call.Method, call.Arguments.String(0), call.Arguments.Get(3).([]string)...)
			_, duplicate := metrics[k]
			assert.False(t, duplicate, "series %+v submitted twice", k)
			metrics[k] = call.Arguments.Get(1).(float64)
		case "Commit":
			committed = true
		}
	}
	assert.True(t, committed, "Commit not called")
	return metrics
}

func poolMetrics(used, committed float64, maxSize *float64, tags ...string) map[metricKey]float64 {
	m := map[metricKey]float64{
		seriesKey("Gauge", "jvm_memory.pool.used", tags...):      used,
		seriesKey("Gauge", "jvm_memory.pool.committed", tags...): committed,
	}
	if maxSize != nil {
		m[seriesKey("Gauge", "jvm_memory.pool.max", tags...)] = *maxSize
	}
	return m
}

func gcMetrics(pauses, fullPauses, pauseTimeSeconds float64, tags ...string) map[metricKey]float64 {
	return map[metricKey]float64{
		seriesKey("Count", "jvm_memory.gc.pauses", tags...):      pauses,
		seriesKey("Count", "jvm_memory.gc.full_pauses", tags...): fullPauses,
		seriesKey("Count", "jvm_memory.gc.pause_time", tags...):  pauseTimeSeconds,
	}
}

func merge(series ...map[metricKey]float64) map[metricKey]float64 {
	out := make(map[metricKey]float64)
	for _, m := range series {
		maps.Copy(out, m)
	}
	return out
}

func ptr(v float64) *float64 {
	return &v
}

func TestNormalizePoolName(t *testing.T) {
	for _, tc := range []struct {
		name       string
		normalized string
		poolType   string
	}{
		{"G1 Eden Space", "g1_eden_space", "heap"},
		{"G1 Old Gen", "g1_old_gen", "heap"},
		{"G1 Survivor Space", "g1_survivor_space", "heap"},
		{"PS Old Gen", "ps_old_gen", "heap"},
		{"Tenured Gen", "tenured_gen", "heap"},
		{"ZGC Old Generation", "zgc_old_generation", "heap"},
		{"Shenandoah", "shenandoah", "heap"},
		{"Metaspace", "metaspace", "non_heap"},
		{"Compressed Class Space", "compressed_class_space", "non_heap"},
		{"Code Cache", "code_cache", "non_heap"},
		{"CodeHeap 'non-nmethods'", "codeheap_non_nmethods", "non_heap"},
		{"CodeHeap 'profiled nmethods'", "codeheap_profiled_nmethods", "non_heap"},
		{"CodeHeap 'non-profiled nmethods'", "codeheap_non_profiled_nmethods", "non_heap"},
		{"  --Weird__Name!! ", "weird_name", "heap"},
		{"Metaspace2", "metaspace2", "heap"},
		{"Ünïcode Pool", "n_code_pool", "heap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			normalized := normalizePoolName(tc.name)
			assert.Equal(t, tc.normalized, normalized)
			assert.Equal(t, tc.poolType, poolType(normalized))
		})
	}

	assert.Empty(t, normalizePoolName(""))
	assert.Empty(t, normalizePoolName("' - '"))
}

func TestPoolStaleness(t *testing.T) {
	tc := newTestCheck(t, "{}")
	require.Equal(t, 15*time.Second, tc.check.Interval())

	pool := func(name string, used uint64, lastUpdate int64) model.PoolStats {
		return model.PoolStats{Name: name, Init: model.UndefinedSize, Used: used, Committed: 2 * used, Max: model.UndefinedSize, LastUpdate: lastUpdate}
	}
	edenTags := []string{"pool:g1_eden_space", "pool_type:heap"}
	oldTags := []string{"pool:g1_old_gen", "pool_type:heap"}
	metaTags := []string{"pool:metaspace", "pool_type:non_heap"}

	// First run: the previous run is assumed to have happened one interval ago.
	edenUpdate, oldUpdate := tc.at(-10*time.Second), tc.at(-15*time.Second)
	tc.sysProbe.set(http.StatusOK, []model.JVMStats{{
		PID: 42,
		Pools: []model.PoolStats{
			pool("G1 Eden Space", 10, edenUpdate),                 // fresh
			pool("G1 Old Gen", 20, oldUpdate),                     // exactly one interval ago: stale
			pool("Metaspace", 30, tc.at(-20*time.Second)),         // stale
			pool("' '", 40, tc.at(-1*time.Second)),                // fresh but unusable name
			pool("CodeHeap 'non-nmethods'", 50, tc.at(time.Hour)), // in the future: fresh
		},
	}})
	assert.Equal(t, merge(
		poolMetrics(10, 20, nil, edenTags...),
		poolMetrics(50, 100, nil, "pool:codeheap_non_nmethods", "pool_type:non_heap"),
		gcMetrics(0, 0, 0),
	), tc.run(t))

	// Second run: only the pools updated since the start of the first run are reported.
	tc.advance(15 * time.Second)
	tc.sysProbe.set(http.StatusOK, []model.JVMStats{{
		PID: 42,
		Pools: []model.PoolStats{
			pool("G1 Eden Space", 10, edenUpdate),         // already reported
			pool("G1 Old Gen", 21, tc.at(-5*time.Second)), // updated during the interval
			pool("Metaspace", 31, tc.at(-15*time.Second)), // updated right when the previous run started
		},
	}})
	assert.Equal(t, merge(
		poolMetrics(21, 42, nil, oldTags...),
		gcMetrics(0, 0, 0),
	), tc.run(t))

	// Failed run: nothing is submitted and the reference time is kept.
	tc.advance(15 * time.Second)
	tc.sysProbe.set(http.StatusInternalServerError, nil)
	tc.sender.ResetCalls()
	require.Error(t, tc.check.Run())
	assert.Empty(t, tc.sender.Calls)

	// Next successful run: pools updated since the start of the last
	// successful run, including during the failed one, are reported.
	tc.advance(15 * time.Second)
	tc.sysProbe.set(http.StatusOK, []model.JVMStats{{
		PID: 42,
		Pools: []model.PoolStats{
			pool("G1 Eden Space", 12, tc.at(-25*time.Second)), // during the failed run interval
			pool("G1 Old Gen", 21, tc.at(-35*time.Second)),    // already reported
			pool("Metaspace", 32, tc.at(-1*time.Second)),
		},
	}})
	assert.Equal(t, merge(
		poolMetrics(12, 24, nil, edenTags...),
		poolMetrics(32, 64, nil, metaTags...),
		gcMetrics(0, 0, 0),
	), tc.run(t))
}

func TestMinCollectionInterval(t *testing.T) {
	tc := newTestCheck(t, "min_collection_interval: 60")
	require.Equal(t, time.Minute, tc.check.Interval())

	tc.sysProbe.set(http.StatusOK, []model.JVMStats{{
		PID: 42,
		Pools: []model.PoolStats{
			{Name: "G1 Eden Space", Used: 1, Committed: 2, Max: 3, LastUpdate: tc.at(-50 * time.Second)},
			{Name: "G1 Old Gen", Used: 1, Committed: 2, Max: 3, LastUpdate: tc.at(-70 * time.Second)},
		},
	}})
	assert.Equal(t, merge(
		poolMetrics(1, 2, ptr(3), "pool:g1_eden_space", "pool_type:heap"),
		gcMetrics(0, 0, 0),
	), tc.run(t))
}

func TestAggregationAndTags(t *testing.T) {
	tc := newTestCheck(t, "{}")

	const containerA, containerB = "container-a", "container-b"
	tc.tagger.SetTags(types.NewEntityID(types.ContainerID, containerA), "test", []string{"kube_namespace:ns"}, []string{"pod_name:pod-a"}, []string{"container_id:" + containerA}, nil)
	tc.tagger.SetTags(types.NewEntityID(types.ContainerID, containerB), "test", []string{"kube_namespace:ns"}, []string{"pod_name:pod-b"}, []string{"container_id:" + containerB}, nil)
	tagsA := []string{"kube_namespace:ns", "pod_name:pod-a", "container_id:" + containerA}
	tagsB := []string{"kube_namespace:ns", "pod_name:pod-b", "container_id:" + containerB}

	fresh := tc.at(-time.Second)
	stale := tc.at(-time.Minute)
	jvms := func(gc1, gc2, gc3, gc4 model.GCStats) []model.JVMStats {
		return []model.JVMStats{
			{
				PID:         100,
				ContainerID: containerA,
				Pools: []model.PoolStats{
					{Name: "G1 Eden Space", Init: 1, Used: 10, Committed: 100, Max: model.UndefinedSize, LastUpdate: fresh},
					{Name: "G1 Old Gen", Init: 1, Used: 20, Committed: 200, Max: 1000, LastUpdate: fresh},
					{Name: "Metaspace", Init: model.UndefinedSize, Used: 30, Committed: 300, Max: 3000, LastUpdate: fresh},
					{Name: "Compressed Class Space", Used: 40, Committed: 400, Max: 4000, LastUpdate: fresh},
					{Name: "CodeHeap 'non-nmethods'", Used: 50, Committed: 500, Max: 5000, LastUpdate: stale},
				},
				GC: gc1,
			},
			{
				PID:         101,
				ContainerID: containerA,
				Pools: []model.PoolStats{
					{Name: "G1 Eden Space", Init: 1, Used: 11, Committed: 110, Max: model.UndefinedSize, LastUpdate: fresh},
					{Name: "G1 Old Gen", Init: 1, Used: 21, Committed: 210, Max: 2000, LastUpdate: fresh},
					{Name: "Metaspace", Init: model.UndefinedSize, Used: 31, Committed: 310, Max: model.UndefinedSize, LastUpdate: fresh},
					// A stale row contributes its latest known value to a group
					// that has a fresh row.
					{Name: "Compressed Class Space", Used: 41, Committed: 410, Max: model.UndefinedSize, LastUpdate: stale},
					// A group without any fresh row is not reported.
					{Name: "CodeHeap 'non-nmethods'", Used: 51, Committed: 510, Max: 5100, LastUpdate: stale},
				},
				GC: gc2,
			},
			{
				PID:         200,
				ContainerID: containerB,
				Pools: []model.PoolStats{
					{Name: "G1 Eden Space", Used: 5, Committed: 50, Max: 500, LastUpdate: fresh},
					{Name: "G1 Old Gen", Used: 6, Committed: 60, Max: 600, LastUpdate: stale},
				},
				GC: gc3,
			},
			{
				PID: 300, // host JVM
				Pools: []model.PoolStats{
					{Name: "G1 Eden Space", Used: 7, Committed: 70, Max: model.UndefinedSize, LastUpdate: fresh},
				},
				GC: gc4,
			},
		}
	}

	withPool := func(tags []string, pool, poolType string) []string {
		return append(slices.Clone(tags), "pool:"+pool, "pool_type:"+poolType)
	}
	expectedPools := merge(
		poolMetrics(21, 210, nil, withPool(tagsA, "g1_eden_space", "heap")...),
		poolMetrics(41, 410, ptr(3000), withPool(tagsA, "g1_old_gen", "heap")...),
		poolMetrics(61, 610, nil, withPool(tagsA, "metaspace", "non_heap")...),
		poolMetrics(81, 810, nil, withPool(tagsA, "compressed_class_space", "non_heap")...),
		poolMetrics(5, 50, ptr(500), withPool(tagsB, "g1_eden_space", "heap")...),
		poolMetrics(7, 70, nil, "pool:g1_eden_space", "pool_type:heap"),
	)

	// First run: GC counters are only recorded as baselines, but zero counts
	// are still submitted once per tag set.
	tc.sysProbe.set(http.StatusOK, jvms(
		model.GCStats{Pauses: 10, FullPauses: 1, PauseTimeNs: 1e9},
		model.GCStats{Pauses: 20, FullPauses: 2, PauseTimeNs: 2e9},
		model.GCStats{Pauses: 30, FullPauses: 3, PauseTimeNs: 3e9},
		model.GCStats{},
	))
	assert.Equal(t, merge(
		expectedPools,
		gcMetrics(0, 0, 0, tagsA...),
		gcMetrics(0, 0, 0, tagsB...),
		gcMetrics(0, 0, 0),
	), tc.run(t))

	// Second run: GC deltas of the JVMs sharing a container are summed.
	tc.advance(15 * time.Second)
	fresh = tc.at(-time.Second)
	tc.sysProbe.set(http.StatusOK, jvms(
		model.GCStats{Pauses: 12, FullPauses: 1, PauseTimeNs: 1.25e9},
		model.GCStats{Pauses: 23, FullPauses: 3, PauseTimeNs: 2.5e9},
		model.GCStats{Pauses: 30, FullPauses: 3, PauseTimeNs: 3e9},
		model.GCStats{Pauses: 4, PauseTimeNs: 1e8},
	))
	assert.Equal(t, merge(
		expectedPools,
		gcMetrics(5, 1, 0.75, tagsA...),
		gcMetrics(0, 0, 0, tagsB...),
		gcMetrics(4, 0, 0.1),
	), tc.run(t))

	// Third run: no pool was updated since the second run, only the GC counts
	// are reported.
	tc.advance(15 * time.Second)
	assert.Equal(t, merge(
		gcMetrics(0, 0, 0, tagsA...),
		gcMetrics(0, 0, 0, tagsB...),
		gcMetrics(0, 0, 0),
	), tc.run(t))
}

func TestGCDeltas(t *testing.T) {
	tc := newTestCheck(t, "{}")

	runWith := func(jvms ...model.JVMStats) map[metricKey]float64 {
		tc.advance(15 * time.Second)
		tc.sysProbe.set(http.StatusOK, jvms)
		return tc.run(t)
	}
	jvm := func(pid uint32, pauses, fullPauses, pauseTimeNs uint64) model.JVMStats {
		return model.JVMStats{PID: pid, GC: model.GCStats{Pauses: pauses, FullPauses: fullPauses, PauseTimeNs: pauseTimeNs}}
	}

	// First sight: baseline only.
	assert.Equal(t, gcMetrics(0, 0, 0), runWith(jvm(1, 100, 10, 5e9)))
	// Increment.
	assert.Equal(t, gcMetrics(3, 1, 0.5), runWith(jvm(1, 103, 11, 5.5e9)))
	// No GC since the previous run: zero counts are still submitted.
	assert.Equal(t, gcMetrics(0, 0, 0), runWith(jvm(1, 103, 11, 5.5e9)))
	// PID reused by another JVM: counters decreased, new baseline.
	assert.Equal(t, gcMetrics(0, 0, 0), runWith(jvm(1, 2, 0, 1e8)))
	assert.Equal(t, gcMetrics(1, 0, 0.1), runWith(jvm(1, 3, 0, 2e8)))
	// A decrease of any counter is a PID reuse.
	assert.Equal(t, gcMetrics(0, 0, 0), runWith(jvm(1, 10, 0, 1e8)))
	assert.Equal(t, gcMetrics(0, 0, 0), runWith(jvm(1, 10, 0, 1e8), jvm(2, 50, 5, 1e9)))
	assert.Contains(t, tc.check.gcBaselines, uint32(2))

	// PID 1 is gone: its state is pruned.
	assert.Equal(t, gcMetrics(1, 0, 0), runWith(jvm(2, 51, 5, 1e9)))
	assert.NotContains(t, tc.check.gcBaselines, uint32(1))
	// When PID 1 comes back, it is a new JVM: baseline only, even if its
	// counters are higher than the ones of the previous JVM with that PID.
	assert.Equal(t, gcMetrics(0, 0, 0), runWith(jvm(1, 1000, 100, 1e12), jvm(2, 51, 5, 1e9)))
	assert.Equal(t, gcMetrics(2, 1, 1), runWith(jvm(1, 1001, 101, 1e12+1e9), jvm(2, 52, 5, 1e9)))

	// No JVM at all: nothing submitted, all state pruned.
	assert.Empty(t, runWith())
	assert.Empty(t, tc.check.gcBaselines)
}

func TestUnknownContainerIsSkipped(t *testing.T) {
	tc := newTestCheck(t, "{}")
	const containerID = "c0ffee"
	tags := []string{"container_id:" + containerID}

	runWith := func(pauses uint64) map[metricKey]float64 {
		tc.advance(15 * time.Second)
		tc.sysProbe.set(http.StatusOK, []model.JVMStats{{
			PID:         1,
			ContainerID: containerID,
			Pools:       []model.PoolStats{{Name: "G1 Eden Space", Used: 1, Committed: 2, Max: model.UndefinedSize, LastUpdate: tc.at(-time.Second)}},
			GC:          model.GCStats{Pauses: pauses},
		}})
		return tc.run(t)
	}

	// The tagger does not know the container yet: the JVM is not reported with
	// the JVMs of the host.
	assert.Empty(t, runWith(10))
	assert.Empty(t, tc.check.gcBaselines)

	// Once known, it is reported with its tags.
	tc.tagger.SetTags(types.NewEntityID(types.ContainerID, containerID), "test", nil, nil, tags, nil)
	assert.Equal(t, merge(
		poolMetrics(1, 2, nil, append([]string{"pool:g1_eden_space", "pool_type:heap"}, tags...)...),
		gcMetrics(0, 0, 0, tags...),
	), runWith(12))
	assert.Equal(t, merge(
		poolMetrics(1, 2, nil, append([]string{"pool:g1_eden_space", "pool_type:heap"}, tags...)...),
		gcMetrics(3, 0, 0, tags...),
	), runWith(15))
}
