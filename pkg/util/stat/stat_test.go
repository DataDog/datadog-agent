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

		// Capture the value before cleanup: unblocking Update (below)
		// may overwrite myStat with a zero value.
		val := myStat.Value()

		// Unblock the Update goroutine in case it selected <-t.C and is
		// now blocked on <-s.Aggregated (Process may have exited via
		// <-s.stopped before sending a value). The zero Stat is
		// harmless — Update will set myStat to 0, but we have already
		// captured the real value above.
		select {
		case s.Aggregated <- Stat{}:
		default:
		}

		synctest.Wait()
		assert.NotEqual(t, val, 0)
	})
}
