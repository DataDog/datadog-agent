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
