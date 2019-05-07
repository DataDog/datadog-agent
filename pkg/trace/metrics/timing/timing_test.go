package timing

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"

	"github.com/stretchr/testify/assert"
)

func TestTiming(t *testing.T) {
	assert := assert.New(t)
	stats := &testutil.TestStatsClient{}

	defer func(old metrics.StatsClient) {
		metrics.Client = old
	}(metrics.Client)
	metrics.Client = stats

	set := NewSet(context.TODO(), "counter1")
	set.Time("counter1", time.Now().Add(-2*time.Second))
	set.Time("counter1", time.Now().Add(-3*time.Second))
	set.Report()

	calls := stats.GaugeCalls
	assert.Equal(3, len(calls))
	assert.Equal(2., findCall(assert, calls, "counter1.count").Value)
	assert.Equal(2500., float64(findCall(assert, calls, "counter1.avg").Value), "avg")
	assert.Equal(3000., findCall(assert, calls, "counter1.max").Value, "max")
}

func findCall(assert *assert.Assertions, calls []testutil.StatsClientGaugeArgs, name string) testutil.StatsClientGaugeArgs {
	for _, c := range calls {
		if c.Name == name {
			return c
		}
	}
	assert.Failf("call not found", "key %q missing", name)
	return testutil.StatsClientGaugeArgs{}
}
