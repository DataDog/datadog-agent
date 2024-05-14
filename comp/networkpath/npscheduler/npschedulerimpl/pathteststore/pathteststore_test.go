// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package pathteststore

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler/npschedulerimpl/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
)

// MockTimeNow mocks time.Now
var MockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

func setMockTimeNow(newTime time.Time) {
	timeNow = func() time.Time {
		return newTime
	}
}

func Test_pathtestStore_add(t *testing.T) {
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())

	// GIVEN
	store := NewPathtestStore(10*time.Minute, 1*time.Minute, logger)

	// WHEN
	pt1 := &common.Pathtest{Hostname: "host1", Port: 53}
	pt2 := &common.Pathtest{Hostname: "host2", Port: 53}
	pt3 := &common.Pathtest{Hostname: "host3", Port: 53}
	store.Add(pt1)
	store.Add(pt2)
	store.Add(pt3)

	// THEN
	assert.Equal(t, 3, len(store.pathtestContexts))

	pt1Ctx := store.pathtestContexts[pt1.GetHash()]
	pt2Ctx := store.pathtestContexts[pt2.GetHash()]
	assert.Equal(t, *pt1, *pt1Ctx.Pathtest)
	assert.Equal(t, *pt2, *pt2Ctx.Pathtest)
}

func Test_pathtestStore_flush(t *testing.T) {
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	timeNow = MockTimeNow
	runDurationFromDisc := 10 * time.Minute
	runInterval := 1 * time.Minute

	// GIVEN
	store := NewPathtestStore(runDurationFromDisc, runInterval, logger)

	// WHEN
	pt := &common.Pathtest{Hostname: "host1", Port: 53}
	store.Add(pt)

	// THEN
	assert.Equal(t, 1, len(store.pathtestContexts))

	ptCtx := store.pathtestContexts[pt.GetHash()]

	assert.Equal(t, MockTimeNow(), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)

	// test first flush, it should increment nextRunTime
	flushTime1 := MockTimeNow().Add(10 * time.Second)
	setMockTimeNow(flushTime1)
	// TODO: check flush results
	store.Flush()
	ptCtx = store.pathtestContexts[pt.GetHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)
	assert.Equal(t, time.Duration(0), ptCtx.lastFlushInterval)

	// skip flush if nextRunTime is not reached yet
	flushTime2 := MockTimeNow().Add(20 * time.Second)
	setMockTimeNow(flushTime2)
	store.Flush()
	ptCtx = store.pathtestContexts[pt.GetHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)
	assert.Equal(t, time.Duration(0), ptCtx.lastFlushInterval)

	// test flush, it should increment nextRunTime
	flushTime3 := MockTimeNow().Add(70 * time.Second)
	setMockTimeNow(flushTime3)
	store.Flush()
	ptCtx = store.pathtestContexts[pt.GetHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*2), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)
	assert.Equal(t, 1*time.Minute, ptCtx.lastFlushInterval)

	// test add new Pathtest after nextRunTime is reached
	// it should reset runUntilTime
	flushTime4 := MockTimeNow().Add(80 * time.Second)
	setMockTimeNow(flushTime4)
	store.Add(pt)
	ptCtx = store.pathtestContexts[pt.GetHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*2), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc+80*time.Second), ptCtx.runUntilTime)
	assert.Equal(t, 1*time.Minute, ptCtx.lastFlushInterval)

	// test flush, it should increment nextRunTime
	flushTime5 := MockTimeNow().Add(120 * time.Second)
	setMockTimeNow(flushTime5)
	store.Flush()
	ptCtx = store.pathtestContexts[pt.GetHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*3), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc+80*time.Second), ptCtx.runUntilTime)
	assert.Equal(t, 50*time.Second, ptCtx.lastFlushInterval)

	// test flush before runUntilTime, it should NOT delete Pathtest entry
	flushTime6 := MockTimeNow().Add((600 + 70) * time.Second)
	setMockTimeNow(flushTime6)
	store.Flush()
	ptCtx = store.pathtestContexts[pt.GetHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*4), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc+80*time.Second), ptCtx.runUntilTime)

	// test flush after runUntilTime, it should delete Pathtest entry
	flushTime7 := MockTimeNow().Add((600 + 90) * time.Second)
	setMockTimeNow(flushTime7)
	store.Flush()
	assert.Equal(t, 0, len(store.pathtestContexts))
}
