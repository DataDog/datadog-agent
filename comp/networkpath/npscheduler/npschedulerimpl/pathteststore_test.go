package npschedulerimpl

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
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
	store := newPathtestStore(DefaultFlushTickerInterval, DefaultPathtestRunDurationFromDiscovery, DefaultPathtestRunInterval, logger)

	// WHEN
	pt1 := &pathtest{hostname: "host1", port: 53}
	pt2 := &pathtest{hostname: "host2", port: 53}
	pt3 := &pathtest{hostname: "host3", port: 53}
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
	pt := &pathtest{hostname: "host1", port: 53}
	store.add(pt)

	// THEN
	assert.Equal(t, 1, len(store.pathtestContexts))

	wrappedFlow := store.pathtestContexts[pt.getHash()]

	assert.Equal(t, MockTimeNow(), wrappedFlow.nextRunTime)
	assert.Equal(t, MockTimeNow().Add(runDurationFromDisc), wrappedFlow.runUntilTime)

	//// test first flush
	//// set flush time
	//flushTime1 := MockTimeNow().Add(10 * time.Second)
	//setMockTimeNow(flushTime1)
	//store.flush()
	//wrappedFlow = store.pathtestContexts[pt.getHash()]
	//assert.Equal(t, MockTimeNow().Add(store.flowFlushInterval), wrappedFlow.nextRunTime)
	//assert.Equal(t, MockTimeNow().Add(10*time.Second), wrappedFlow.lastSuccessfulFlush)
	//
	//// test skip flush if nextRunTime is not reached yet
	//flushTime2 := MockTimeNow().Add(15 * time.Second)
	//setMockTimeNow(flushTime2)
	//store.flush()
	//wrappedFlow = store.pathtestContexts[pt.getHash()]
	//assert.Equal(t, MockTimeNow().Add(store.flowFlushInterval), wrappedFlow.nextRunTime)
	//assert.Equal(t, MockTimeNow().Add(10*time.Second), wrappedFlow.lastSuccessfulFlush)
	//
	//// test flush with no new flow after nextRunTime is reached
	//flushTime3 := MockTimeNow().Add(store.flowFlushInterval + (1 * time.Second))
	//setMockTimeNow(flushTime3)
	//store.flush()
	//wrappedFlow = store.pathtestContexts[pt.getHash()]
	//assert.Equal(t, MockTimeNow().Add(store.flowFlushInterval*2), wrappedFlow.nextRunTime)
	//// lastSuccessfulFlush time doesn't change because there is no new flow
	//assert.Equal(t, MockTimeNow().Add(10*time.Second), wrappedFlow.lastSuccessfulFlush)
	//
	//// test flush with new flow after nextRunTime is reached
	//flushTime4 := MockTimeNow().Add(store.flowFlushInterval*2 + (1 * time.Second))
	//setMockTimeNow(flushTime4)
	//store.add(flow)
	//store.flush()
	//wrappedFlow = store.pathtestContexts[pt.getHash()]
	//assert.Equal(t, MockTimeNow().Add(store.flowFlushInterval*3), wrappedFlow.nextRunTime)
	//assert.Equal(t, flushTime4, wrappedFlow.lastSuccessfulFlush)
	//
	//// test flush with TTL reached (now+ttl is equal last successful flush) to clean up entry
	//flushTime5 := flushTime4.Add(flowContextTTL + 1*time.Second)
	//setMockTimeNow(flushTime5)
	//store.flush()
	//_, ok := store.pathtestContexts[pt.getHash()]
	//assert.False(t, ok)
	//
	//// test flush with TTL reached (now+ttl is after last successful flush) to clean up entry
	//setMockTimeNow(MockTimeNow())
	//store.add(flow)
	//store.flush()
	//flushTime6 := MockTimeNow().Add(flowContextTTL + 1*time.Second)
	//setMockTimeNow(flushTime6)
	//store.flush()
	//_, ok = store.pathtestContexts[pt.getHash()]
	//assert.False(t, ok)
}
