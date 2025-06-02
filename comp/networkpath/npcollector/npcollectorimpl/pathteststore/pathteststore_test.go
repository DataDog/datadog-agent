// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package pathteststore

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
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
			l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
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
