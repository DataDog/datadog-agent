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
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler/npschedulerimpl"
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
	store := newPathtestStore(npschedulerimpl.DefaultFlushTickerInterval, 10*time.Minute, 1*time.Minute, logger)

	// WHEN
	pt1 := &Pathtest{Hostname: "host1", Port: 53}
	pt2 := &Pathtest{Hostname: "host2", Port: 53}
	pt3 := &Pathtest{Hostname: "host3", Port: 53}
	store.add(pt1)
	store.add(pt2)
	store.add(pt3)

	// THEN
	assert.Equal(t, 3, len(store.pathtestContexts))

	pt1Ctx := store.pathtestContexts[pt1.getHash()]
	pt2Ctx := store.pathtestContexts[pt2.getHash()]
	assert.Equal(t, *pt1, *pt1Ctx.pathtest)
	assert.Equal(t, *pt2, *pt2Ctx.pathtest)
}

func Test_pathtestStore_flush(t *testing.T) {
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	timeNow = MockTimeNow
	flushTickerInterval := 10 * time.Second
	runDurationFromDisc := 10 * time.Minute
	runInterval := 1 * time.Minute

	// GIVEN
	store := newPathtestStore(flushTickerInterval, runDurationFromDisc, runInterval, logger)

	// WHEN
	pt := &Pathtest{Hostname: "host1", Port: 53}
	store.add(pt)

	// THEN
	assert.Equal(t, 1, len(store.pathtestContexts))

	ptCtx := store.pathtestContexts[pt.getHash()]

	assert.Equal(t, MockTimeNow(), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)

	// test first flush, it should increment nextRunTime
	flushTime1 := MockTimeNow().Add(10 * time.Second)
	setMockTimeNow(flushTime1)
	// TODO: check flush results
	store.flush()
	ptCtx = store.pathtestContexts[pt.getHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)

	// skip flush if nextRunTime is not reached yet
	flushTime2 := MockTimeNow().Add(20 * time.Second)
	setMockTimeNow(flushTime2)
	store.flush()
	ptCtx = store.pathtestContexts[pt.getHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)

	// test flush, it should increment nextRunTime
	flushTime3 := MockTimeNow().Add(70 * time.Second)
	setMockTimeNow(flushTime3)
	store.flush()
	ptCtx = store.pathtestContexts[pt.getHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*2), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), ptCtx.runUntilTime)

	// test add new Pathtest after nextRunTime is reached
	// it should reset runUntilTime
	flushTime4 := MockTimeNow().Add(80 * time.Second)
	setMockTimeNow(flushTime4)
	store.add(pt)
	ptCtx = store.pathtestContexts[pt.getHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*2), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc+80*time.Second), ptCtx.runUntilTime)

	// test flush, it should increment nextRunTime
	flushTime5 := MockTimeNow().Add(120 * time.Second)
	setMockTimeNow(flushTime5)
	store.flush()
	ptCtx = store.pathtestContexts[pt.getHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*3), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc+80*time.Second), ptCtx.runUntilTime)

	// test flush before runUntilTime, it should NOT delete Pathtest entry
	flushTime6 := MockTimeNow().Add((600 + 70) * time.Second)
	setMockTimeNow(flushTime6)
	store.flush()
	ptCtx = store.pathtestContexts[pt.getHash()]
	assert.Equal(t, MockTimeNow().Add(store.pathtestInterval*4), ptCtx.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc+80*time.Second), ptCtx.runUntilTime)

	// test flush after runUntilTime, it should delete Pathtest entry
	flushTime7 := MockTimeNow().Add((600 + 90) * time.Second)
	setMockTimeNow(flushTime7)
	store.flush()
	assert.Equal(t, 0, len(store.pathtestContexts))
}
