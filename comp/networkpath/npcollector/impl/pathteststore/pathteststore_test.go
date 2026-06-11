// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package pathteststore

import (
	"bufio"
	"bytes"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
)

var mockTimeJan2 = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}()

var mockedTimeNow = time.Now()

func mockTimeNow() time.Time {
	return mockedTimeNow
}
func setMockTimeNow(newTime time.Time) {
	mockedTimeNow = newTime
}

func Test_pathtestStore_add(t *testing.T) {
	testcases := []struct {
		name             string
		initialSize      int
		pathtests        []*common.Pathtest
		expectedSize     int
		expectedLogCount int
		overrideLogTime  bool
	}{
		{
			name:        "Store not full",
			initialSize: 3,
			pathtests: []*common.Pathtest{
				{Hostname: "host1", Port: 53},
				{Hostname: "host2", Port: 53},
				{Hostname: "host3", Port: 53},
			},
			expectedSize:     3,
			expectedLogCount: 0,
			overrideLogTime:  false,
		},
		{
			name:        "Store full, one warning",
			initialSize: 2,
			pathtests: []*common.Pathtest{
				{Hostname: "host1", Port: 53},
				{Hostname: "host2", Port: 53},
				{Hostname: "host3", Port: 53},
				{Hostname: "host4", Port: 53},
			},
			expectedSize:     2,
			expectedLogCount: 1,
			overrideLogTime:  false,
		},
		{
			name:        "Store full, multiple warnings",
			initialSize: 2,
			pathtests: []*common.Pathtest{
				{Hostname: "host1", Port: 53},
				{Hostname: "host2", Port: 53},
				{Hostname: "host3", Port: 53},
				{Hostname: "host4", Port: 53},
			},
			expectedSize:     2,
			expectedLogCount: 2,
			overrideLogTime:  true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
			assert.Nil(t, err)
			utillog.SetupLogger(l, "debug")

			config := Config{
				ContextsLimit: tc.initialSize,
				TTL:           10 * time.Minute,
				Interval:      1 * time.Minute,
			}

			setMockTimeNow(mockTimeJan2)
			store := NewPathtestStore(config, l, &statsd.NoOpClient{}, mockTimeNow)

			for _, pt := range tc.pathtests {
				store.Add(pt)
				if tc.overrideLogTime {
					store.contextsMutex.Lock()
					store.lastContextWarning = mockTimeJan2.Add(5 * time.Minute)
					store.contextsMutex.Unlock()
				}
			}

			// TEST START/STOP using logs
			l.Close() // We need to first close the logger to avoid a race-cond between seelog and out test when calling w.Flush()
			w.Flush()
			logs := b.String()

			assert.Equal(t, tc.expectedSize, len(store.contexts))
			assert.Equal(t, tc.expectedLogCount, strings.Count(logs, "Pathteststore is full"), logs)
		})
	}
}

func Test_pathtestStore_add_when_full(t *testing.T) {
	logger := logmock.New(t)

	// GIVEN
	config := Config{
		ContextsLimit: 2,
		TTL:           10 * time.Minute,
		Interval:      1 * time.Minute,
	}
	setMockTimeNow(mockTimeJan2)
	store := NewPathtestStore(config, logger, &statsd.NoOpClient{}, mockTimeNow)

	// WHEN
	pt1 := &common.Pathtest{Hostname: "host1", Port: 53}
	pt2 := &common.Pathtest{Hostname: "host2", Port: 53}
	pt3 := &common.Pathtest{Hostname: "host3", Port: 53}
	store.Add(pt1)
	store.Add(pt2)
	store.Add(pt3)

	// THEN
	assert.Equal(t, 2, len(store.contexts))

	pt1Ctx := store.contexts[pt1.GetHash()]
	pt2Ctx := store.contexts[pt2.GetHash()]
	assert.Equal(t, *pt1, *pt1Ctx.Pathtest)
	assert.Equal(t, *pt2, *pt2Ctx.Pathtest)
}

func Test_pathtestStore_flush(t *testing.T) {
	logger := logmock.New(t)
	setMockTimeNow(mockTimeJan2)

	// GIVEN
	config := Config{
		ContextsLimit: 10,
		TTL:           10 * time.Minute,
		Interval:      1 * time.Minute,
	}
	store := NewPathtestStore(config, logger, &statsd.NoOpClient{}, mockTimeNow)

	// WHEN
	pt := &common.Pathtest{Hostname: "host1", Port: 53}
	store.Add(pt)

	// THEN
	assert.Equal(t, 1, len(store.contexts))

	ptCtx := store.contexts[pt.GetHash()]

	assert.Equal(t, mockTimeJan2, ptCtx.nextRun)
	assert.Equal(t, mockTimeJan2.Add(config.TTL), ptCtx.runUntil)

	// test first flush, it should increment nextRun
	flushTime1 := mockTimeJan2.Add(10 * time.Second)
	setMockTimeNow(flushTime1)
	// TODO: check flush results
	store.Flush()
	ptCtx = store.contexts[pt.GetHash()]
	assert.Equal(t, mockTimeJan2.Add(config.Interval), ptCtx.nextRun)
	assert.Equal(t, mockTimeJan2.Add(10*time.Minute), ptCtx.runUntil)
	assert.Equal(t, time.Duration(0), ptCtx.lastFlushInterval)

	// skip flush if nextRun is not reached yet
	flushTime2 := mockTimeJan2.Add(20 * time.Second)
	setMockTimeNow(flushTime2)
	store.Flush()
	ptCtx = store.contexts[pt.GetHash()]
	assert.Equal(t, mockTimeJan2.Add(config.Interval), ptCtx.nextRun)
	assert.Equal(t, mockTimeJan2.Add(10*time.Minute), ptCtx.runUntil)
	assert.Equal(t, time.Duration(0), ptCtx.lastFlushInterval)

	// test flush, it should increment nextRun
	flushTime3 := mockTimeJan2.Add(70 * time.Second)
	setMockTimeNow(flushTime3)
	store.Flush()
	ptCtx = store.contexts[pt.GetHash()]
	assert.Equal(t, mockTimeJan2.Add(config.Interval*2), ptCtx.nextRun)
	assert.Equal(t, mockTimeJan2.Add(10*time.Minute), ptCtx.runUntil)
	assert.Equal(t, 1*time.Minute, ptCtx.lastFlushInterval)

	// test add new Pathtest after nextRun is reached
	// it should reset runUntil
	flushTime4 := mockTimeJan2.Add(80 * time.Second)
	setMockTimeNow(flushTime4)
	store.Add(pt)
	ptCtx = store.contexts[pt.GetHash()]
	assert.Equal(t, mockTimeJan2.Add(config.Interval*2), ptCtx.nextRun)
	assert.Equal(t, mockTimeJan2.Add(10*time.Minute+80*time.Second), ptCtx.runUntil)
	assert.Equal(t, 1*time.Minute, ptCtx.lastFlushInterval)

	// test flush, it should increment nextRun
	flushTime5 := mockTimeJan2.Add(120 * time.Second)
	setMockTimeNow(flushTime5)
	store.Flush()
	ptCtx = store.contexts[pt.GetHash()]
	assert.Equal(t, mockTimeJan2.Add(config.Interval*3), ptCtx.nextRun)
	assert.Equal(t, mockTimeJan2.Add(10*time.Minute+80*time.Second), ptCtx.runUntil)
	assert.Equal(t, 50*time.Second, ptCtx.lastFlushInterval)

	// test flush before runUntil, it should NOT delete Pathtest entry
	flushTime6 := mockTimeJan2.Add((600 + 70) * time.Second)
	setMockTimeNow(flushTime6)
	store.Flush()
	ptCtx = store.contexts[pt.GetHash()]
	assert.Equal(t, mockTimeJan2.Add(config.Interval*4), ptCtx.nextRun)
	assert.Equal(t, mockTimeJan2.Add(10*time.Minute+80*time.Second), ptCtx.runUntil)

	// test flush after runUntil, it should delete Pathtest entry
	flushTime7 := mockTimeJan2.Add((600 + 90) * time.Second)
	setMockTimeNow(flushTime7)
	store.Flush()
	assert.Equal(t, 0, len(store.contexts))
}

type rateLimitTestStep struct {
	timestep           time.Duration
	addCount           int
	expectedFlushCount int
}

func Test_pathtestStore_rate_limit_circuit_breaker(t *testing.T) {
	testcases := []struct {
		name             string
		maxPerMinute     int
		maxBurstDuration time.Duration
		sequence         []rateLimitTestStep
	}{
		{
			name:             "no rate limit",
			maxPerMinute:     0,
			maxBurstDuration: 30 * time.Second,
			sequence: []rateLimitTestStep{
				{timestep: 10 * time.Second, addCount: 1, expectedFlushCount: 1},
				{timestep: 10 * time.Second, addCount: 20, expectedFlushCount: 20},
			},
		},
		{
			name:             "burst with rate limit",
			maxPerMinute:     60,
			maxBurstDuration: 30 * time.Second,
			sequence: []rateLimitTestStep{
				// drain the burst first
				{timestep: 10 * time.Second, addCount: 30, expectedFlushCount: 30},
				{timestep: 10 * time.Second, addCount: 30, expectedFlushCount: 10},
				{timestep: 10 * time.Second, addCount: 0, expectedFlushCount: 10},
				{timestep: 10 * time.Second, addCount: 0, expectedFlushCount: 10},
				{timestep: 10 * time.Second, addCount: 0, expectedFlushCount: 0},
			},
		},
		{
			name:             "underneath rate limit",
			maxPerMinute:     60,
			maxBurstDuration: time.Minute,
			sequence: []rateLimitTestStep{
				{timestep: 10 * time.Second, addCount: 1, expectedFlushCount: 1},
				{timestep: 10 * time.Second, addCount: 2, expectedFlushCount: 2},
				{timestep: 10 * time.Second, addCount: 3, expectedFlushCount: 3},
				{timestep: 10 * time.Second, addCount: 4, expectedFlushCount: 4},
			},
		},
		{
			name:             "slight bump over the limit gets flushed in the next one",
			maxPerMinute:     60,
			maxBurstDuration: 10 * time.Second,
			sequence: []rateLimitTestStep{
				// drain the burst first
				{timestep: 10 * time.Second, addCount: 10, expectedFlushCount: 10},
				{timestep: 10 * time.Second, addCount: 1, expectedFlushCount: 1},
				{timestep: 10 * time.Second, addCount: 12, expectedFlushCount: 10},
				{timestep: 10 * time.Second, addCount: 3, expectedFlushCount: 3 + 2},
				{timestep: 10 * time.Second, addCount: 4, expectedFlushCount: 4},
			},
		},
		{
			name:             "big rate limit, quick timestep",
			maxPerMinute:     600,
			maxBurstDuration: 1 * time.Second,
			sequence: []rateLimitTestStep{
				// drain the burst first
				{timestep: 10 * time.Second, addCount: 10, expectedFlushCount: 10},
				{timestep: 1 * time.Second, addCount: 15, expectedFlushCount: 10},
				{timestep: 1 * time.Second, addCount: 0, expectedFlushCount: 5},
				{timestep: 1 * time.Second, addCount: 0, expectedFlushCount: 0},
			},
		},
		{
			name:             "past unused budget doesn't cause a spike",
			maxPerMinute:     60,
			maxBurstDuration: 10 * time.Second,
			sequence: []rateLimitTestStep{
				{timestep: 10 * time.Second, addCount: 0, expectedFlushCount: 0},
				{timestep: 10 * time.Second, addCount: 20, expectedFlushCount: 10},
				{timestep: 10 * time.Second, addCount: 0, expectedFlushCount: 10},
				{timestep: 10 * time.Second, addCount: 0, expectedFlushCount: 0},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			logger := logmock.New(t)

			// GIVEN
			config := Config{
				// big ContextsLimit so it doesn't run out of space
				ContextsLimit: 100,
				// want a long TTL/interval so pathtests run exactly once
				TTL:      10 * time.Minute,
				Interval: 10 * time.Minute,
				// set by the test case
				MaxPerMinute:     tc.maxPerMinute,
				MaxBurstDuration: tc.maxBurstDuration,
			}

			now := time.Now()
			setMockTimeNow(now)
			store := NewPathtestStore(config, logger, &statsd.NoOpClient{}, mockTimeNow)

			for iStep, step := range tc.sequence {
				for iAdd := range step.addCount {
					// make a unique pathtest so that they don't get deduplicated
					store.Add(&common.Pathtest{
						Hostname: fmt.Sprintf("host-%d-%d", iStep, iAdd),
						Port:     80,
					})
				}

				now = now.Add(step.timestep)
				setMockTimeNow(now)
				flushedPathtests := store.Flush()
				assert.Len(t, flushedPathtests, step.expectedFlushCount, "flushed pathtest step %d", iStep)
			}

		})
	}

}

// Test_pathtestStore_origin_rate_limiter_selection verifies that when
// OriginRateLimits contains an override for OriginNetworkDevice, NDM pathtests
// are governed by the NDM-specific limiter while CNM pathtests (OriginAgentTraffic)
// still use the default limiter.
func Test_pathtestStore_origin_rate_limiter_selection(t *testing.T) {
	logger := logmock.New(t)

	// Configure a tight default (for CNM) and a more generous NDM override.
	config := Config{
		ContextsLimit:    100,
		TTL:              10 * time.Minute,
		Interval:         10 * time.Minute,
		MaxPerMinute:     60, // default: 1/s, burst = 10s worth = 10
		MaxBurstDuration: 10 * time.Second,
		OriginRateLimits: map[model.OriginType]OriginRateLimit{
			model.OriginNetworkDevice: {
				MaxPerMinute:     600, // NDM: 10/s, burst = 10s worth = 100
				MaxBurstDuration: 10 * time.Second,
			},
		},
	}

	now := time.Now()
	setMockTimeNow(now)
	store := NewPathtestStore(config, logger, &statsd.NoOpClient{}, mockTimeNow)

	// Add 15 CNM pathtests (OriginAgentTraffic / zero value)
	for i := range 15 {
		store.Add(&common.Pathtest{
			Hostname: fmt.Sprintf("cnm-host-%d", i),
			Port:     80,
			Origin:   model.OriginAgentTraffic,
		})
	}

	// Add 15 NDM pathtests (OriginNetworkDevice)
	for i := range 15 {
		store.Add(&common.Pathtest{
			Hostname: fmt.Sprintf("ndm-host-%d", i),
			Port:     80,
			Origin:   model.OriginNetworkDevice,
		})
	}

	// Advance time so all pathtests are eligible to flush (nextRun <= now).
	now = now.Add(10 * time.Second)
	setMockTimeNow(now)
	flushed := store.Flush()

	cnmFlushed := 0
	ndmFlushed := 0
	for _, ctx := range flushed {
		if ctx.Pathtest.Origin == model.OriginNetworkDevice {
			ndmFlushed++
		} else {
			cnmFlushed++
		}
	}

	// Default limiter allows burst of 10 within the first 10 s → CNM capped at 10.
	assert.Equal(t, 10, cnmFlushed, "CNM pathtests should be capped by the default limiter")
	// NDM limiter allows burst of 100 within the first 10 s → all 15 NDM tests should flush.
	assert.Equal(t, 15, ndmFlushed, "NDM pathtests should be governed by the NDM-specific limiter")
}

// Test_pathtestStore_origin_rate_limiter_independence verifies that exhausting the
// NDM rate-limit budget does not prevent CNM pathtests from flushing, and vice versa.
func Test_pathtestStore_origin_rate_limiter_independence(t *testing.T) {
	logger := logmock.New(t)

	config := Config{
		ContextsLimit:    200,
		TTL:              10 * time.Minute,
		Interval:         10 * time.Minute,
		MaxPerMinute:     60, // default (CNM): burst = 10 within 10 s
		MaxBurstDuration: 10 * time.Second,
		OriginRateLimits: map[model.OriginType]OriginRateLimit{
			model.OriginNetworkDevice: {
				MaxPerMinute:     60, // NDM: same rate, independent bucket
				MaxBurstDuration: 10 * time.Second,
			},
		},
	}

	now := time.Now()
	setMockTimeNow(now)
	store := NewPathtestStore(config, logger, &statsd.NoOpClient{}, mockTimeNow)

	// Add 20 NDM pathtests to exhaust the NDM burst budget (max 10 in 10 s).
	for i := range 20 {
		store.Add(&common.Pathtest{
			Hostname: fmt.Sprintf("ndm-host-%d", i),
			Port:     80,
			Origin:   model.OriginNetworkDevice,
		})
	}

	// Add 5 CNM pathtests (well within the default budget).
	for i := range 5 {
		store.Add(&common.Pathtest{
			Hostname: fmt.Sprintf("cnm-host-%d", i),
			Port:     80,
			Origin:   model.OriginAgentTraffic,
		})
	}

	now = now.Add(10 * time.Second)
	setMockTimeNow(now)
	flushed := store.Flush()

	cnmFlushed := 0
	ndmFlushed := 0
	for _, ctx := range flushed {
		if ctx.Pathtest.Origin == model.OriginNetworkDevice {
			ndmFlushed++
		} else {
			cnmFlushed++
		}
	}

	// NDM burst budget is 10; the 10 extra NDM pathtests should be held back.
	assert.Equal(t, 10, ndmFlushed, "NDM budget exhausted: only burst-window worth should flush")
	// CNM uses its own independent budget and is not affected by NDM exhaustion.
	assert.Equal(t, 5, cnmFlushed, "CNM budget must be unaffected by NDM budget exhaustion")
}

// Test_pathtestStore_backward_compat verifies that when no OriginRateLimits override
// is set the store behaves identically to the original single-limiter implementation:
// all pathtests (regardless of origin) share the default budget.
func Test_pathtestStore_backward_compat(t *testing.T) {
	logger := logmock.New(t)

	// No OriginRateLimits — exactly what callers using the old Config shape provide.
	config := Config{
		ContextsLimit:    100,
		TTL:              10 * time.Minute,
		Interval:         10 * time.Minute,
		MaxPerMinute:     60,
		MaxBurstDuration: 10 * time.Second,
		// OriginRateLimits intentionally omitted (nil map).
	}

	now := time.Now()
	setMockTimeNow(now)
	store := NewPathtestStore(config, logger, &statsd.NoOpClient{}, mockTimeNow)

	// Mix of CNM and NDM pathtests — both share the single default limiter.
	for i := range 15 {
		store.Add(&common.Pathtest{
			Hostname: fmt.Sprintf("cnm-host-%d", i),
			Port:     80,
			Origin:   model.OriginAgentTraffic,
		})
	}
	for i := range 15 {
		store.Add(&common.Pathtest{
			Hostname: fmt.Sprintf("ndm-host-%d", i),
			Port:     80,
			Origin:   model.OriginNetworkDevice,
		})
	}

	now = now.Add(10 * time.Second)
	setMockTimeNow(now)
	flushed := store.Flush()

	// Total burst budget = 10 (60/min × 10s/60s_per_min = 10).
	// Combined CNM+NDM must not exceed that budget.
	assert.Len(t, flushed, 10, "without OriginRateLimits overrides, all origins share the default budget")
}

// Test_pathtestStore_add_metadata_union verifies that when a hash collision occurs
// (same dedup key added twice), the store unions the Namespaces and ExporterAddrs
// metadata fields rather than discarding the new values.
func Test_pathtestStore_add_metadata_union(t *testing.T) {
	logger := logmock.New(t)

	config := Config{
		ContextsLimit: 10,
		TTL:           10 * time.Minute,
		Interval:      1 * time.Minute,
	}

	setMockTimeNow(mockTimeJan2)
	store := NewPathtestStore(config, logger, &statsd.NoOpClient{}, mockTimeNow)

	// First Add: Namespaces=["ns1"], ExporterAddrs=[]
	pt1 := &common.Pathtest{
		Hostname: "host1",
		Port:     443,
		Protocol: "TCP",
		Origin:   model.OriginNetworkDevice,
		Metadata: common.PathtestMetadata{
			Namespaces: []string{"ns1"},
		},
	}
	store.Add(pt1)

	// Confirm it was stored.
	hash := pt1.GetHash()
	assert.Equal(t, 1, len(store.contexts))
	ptCtx := store.contexts[hash]
	assert.Equal(t, []string{"ns1"}, ptCtx.Pathtest.Metadata.Namespaces)
	assert.Empty(t, ptCtx.Pathtest.Metadata.ExporterAddrs)

	// Second Add with the same hash: Namespaces=["ns2"], ExporterAddrs=[1.1.1.1, 2.2.2.2]
	pt2 := &common.Pathtest{
		Hostname: "host1",
		Port:     443,
		Protocol: "TCP",
		Origin:   model.OriginNetworkDevice,
		Metadata: common.PathtestMetadata{
			Namespaces:    []string{"ns2"},
			ExporterAddrs: []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2.2.2.2")},
		},
	}
	store.Add(pt2)

	// Still only one context (dedup).
	assert.Equal(t, 1, len(store.contexts))

	// Namespaces must contain both ns1 and ns2 (order not guaranteed).
	ptCtx = store.contexts[hash]
	assert.ElementsMatch(t, []string{"ns1", "ns2"}, ptCtx.Pathtest.Metadata.Namespaces)
	assert.ElementsMatch(t,
		[]netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2.2.2.2")},
		ptCtx.Pathtest.Metadata.ExporterAddrs)

	// Third Add: add overlapping ns1 and a new exporter 3.3.3.3 — no duplicates.
	pt3 := &common.Pathtest{
		Hostname: "host1",
		Port:     443,
		Protocol: "TCP",
		Origin:   model.OriginNetworkDevice,
		Metadata: common.PathtestMetadata{
			Namespaces:    []string{"ns1"},
			ExporterAddrs: []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("3.3.3.3")},
		},
	}
	store.Add(pt3)

	ptCtx = store.contexts[hash]
	assert.ElementsMatch(t, []string{"ns1", "ns2"}, ptCtx.Pathtest.Metadata.Namespaces,
		"duplicate ns1 must not be added again")
	assert.ElementsMatch(t,
		[]netip.Addr{
			netip.MustParseAddr("1.1.1.1"),
			netip.MustParseAddr("2.2.2.2"),
			netip.MustParseAddr("3.3.3.3"),
		},
		ptCtx.Pathtest.Metadata.ExporterAddrs,
		"duplicate 1.1.1.1 must not be added; new 3.3.3.3 must be added")
}
