package timing

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"

	"github.com/stretchr/testify/assert"
)

func TestTiming(t *testing.T) {
	assert := assert.New(t)
	stats := &testutil.TestStatsClient{}

	defer func(old metrics.StatsClient) { metrics.Client = old }(metrics.Client)
	metrics.Client = stats

	t.Run("report", func(t *testing.T) {
		stats.Reset()
		set := NewSet("counter2")
		set.Since("counter1", time.Now().Add(-2*time.Second))
		set.Since("counter1", time.Now().Add(-3*time.Second))
		set.Report()

		calls := stats.CountCalls
		assert.Equal(2, len(calls))
		assert.Equal(2., findCall(assert, calls, "counter1.count").Value)

		calls = stats.GaugeCalls
		assert.Equal(4, len(calls))
		assert.Equal(2500., float64(findCall(assert, calls, "counter1.avg").Value), "avg")
		assert.Equal(3000., findCall(assert, calls, "counter1.max").Value, "max")
	})

	t.Run("autoreport", func(t *testing.T) {
		stats.Reset()
		set := NewSet("counter1")
		set.Since("counter1", time.Now().Add(-1*time.Second))
		stop := set.Autoreport(time.Millisecond)
		time.Sleep(5 * time.Millisecond)
		stop()
		assert.True(len(stats.CountCalls) > 1)
	})

	t.Run("panic", func(t *testing.T) {
		set := NewSet("counter1")
		stop := set.Autoreport(time.Millisecond)
		stop()
		stop()
	})

	t.Run("race", func(t *testing.T) {
		stats.Reset()
		set := NewSet("counter1")
		var wg sync.WaitGroup
		for i := 0; i < 150; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				set.Since("counter1", time.Now().Add(-time.Second))
				set.Since(fmt.Sprintf("%d", rand.Int()), time.Now().Add(-time.Second))
			}()
		}

		for i := 0; i < 150; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				set.Report()
			}()
		}
		wg.Wait()
	})
}

func findCall(assert *assert.Assertions, calls []testutil.MetricsArgs, name string) testutil.MetricsArgs {
	for _, c := range calls {
		if c.Name == name {
			return c
		}
	}
	assert.Failf("call not found", "key %q missing", name)
	return testutil.MetricsArgs{}
}
