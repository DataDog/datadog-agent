// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package timing

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
)

func TestTiming(t *testing.T) {
	assert := assert.New(t)
	stats := &teststatsd.Client{}

	Stop() // https://github.com/DataDog/datadog-agent/issues/13934
	defer func(old metrics.StatsClient) { metrics.Client = old }(metrics.Client)
	metrics.Client = stats
	stopReport = defaultSet.Autoreport(AutoreportInterval)

	t.Run("report", func(t *testing.T) {
		stats.Reset()
		set := NewSet()
		set.Since("counter1", time.Now().Add(-2*time.Second))
		set.Since("counter1", time.Now().Add(-3*time.Second))
		set.Report()

		calls := stats.CountCalls
		assert.Equal(1, len(calls))
		assert.Equal(2., findCall(assert, calls, "counter1.count").Value)

		calls = stats.GaugeCalls
		assert.Equal(2, len(calls))
		assert.Equal(2500., findCall(assert, calls, "counter1.avg").Value, "avg")
		assert.Equal(3000., findCall(assert, calls, "counter1.max").Value, "max")
	})

	t.Run("autoreport", func(t *testing.T) {
		stats.Reset()
		set := NewSet()
		set.Since("counter1", time.Now().Add(-1*time.Second))
		stop := set.Autoreport(time.Millisecond)
		if runtime.GOOS == "windows" {
			time.Sleep(5 * time.Second)
		}
		time.Sleep(5 * time.Millisecond)
		stop()
		assert.True(len(stats.CountCalls) > 1)
	})

	t.Run("panic", func(t *testing.T) {
		set := NewSet()
		stop := set.Autoreport(time.Millisecond)
		stop()
		stop()
	})

	t.Run("race", func(t *testing.T) {
		stats.Reset()
		set := NewSet()
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

func findCall(assert *assert.Assertions, calls []teststatsd.MetricsArgs, name string) teststatsd.MetricsArgs {
	for _, c := range calls {
		if c.Name == name {
			return c
		}
	}
	assert.Failf("call not found", "key %q missing", name)
	return teststatsd.MetricsArgs{}
}
