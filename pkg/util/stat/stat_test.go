// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stat

import (
	"expvar"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateStopsWithNoAggregatedValue(t *testing.T) {
	synctest.Test(t, func(_ *testing.T) {
		s := &Stats{stopped: make(chan struct{}), Aggregated: make(chan Stat)}
		var value expvar.Int
		go s.Update(&value)
		time.Sleep(time.Second)
		synctest.Wait()
		s.Stop()
		synctest.Wait()
	})
}

func TestStats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		myStat := expvar.Int{}

		s, err := NewStats(10)
		require.NoError(t, err)
		stop := sync.OnceFunc(s.Stop)
		defer func() {
			stop()
			synctest.Wait()
		}()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		require.NotNil(t, ticker)

		go s.Process()
		go s.Update(&myStat)

		deadline := time.After(2 * time.Second)

	loop:
		for {
			select {
			case <-ticker.C:
				// send a few events per second
				for i := 0; i < 10; i++ {
					s.StatEvent(int64(i))
				}
			case <-deadline:
				stop()
				break loop
			}
		}

		val := myStat.Value()

		synctest.Wait()
		assert.NotEqual(t, val, 0)
	})
}
