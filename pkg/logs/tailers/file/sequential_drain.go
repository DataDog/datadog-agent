// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// sequentialReaderForceCloseGrace is the extra time after max_drain before the
// watchdog force-closes a reader that is still blocked.
const sequentialReaderForceCloseGrace = 2 * time.Second

// BeginSequentialDrain starts a post-rotation drain with quiet-period and max-drain
// semantics. close_timeout is not used on this path.
func (t *Tailer) BeginSequentialDrain(quietPeriod, maxDrain time.Duration) {
	t.didFileRotate.Store(true)
	t.file.Source.RemoveInput(t.file.Path)

	t.sequentialDrainMu.Lock()
	t.sequentialDrainActive = true
	t.emptySince = nil
	t.sequentialQuietPeriod = quietPeriod
	t.sequentialMaxDrain = maxDrain
	t.sequentialDrainStart = time.Now()
	t.sequentialDrainMu.Unlock()

	go t.runSequentialDrainWatchdog()
}

func (t *Tailer) noteSequentialRead(n int) {
	t.sequentialDrainMu.Lock()
	defer t.sequentialDrainMu.Unlock()
	if !t.sequentialDrainActive {
		return
	}
	if n == 0 {
		if t.emptySince == nil {
			now := time.Now()
			t.emptySince = &now
		}
		return
	}
	t.emptySince = nil
}

func (t *Tailer) runSequentialDrainWatchdog() {
	pollInterval := t.sleepDuration
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	maxDrainEscalated := false
	for {
		if t.IsReaderClosed() {
			t.finishSequentialDrainWatchdog(maxDrainEscalated)
			return
		}

		<-ticker.C

		t.sequentialDrainMu.Lock()
		quietReady := false
		if t.emptySince != nil {
			quietReady = time.Since(*t.emptySince) >= t.sequentialQuietPeriod
		}
		maxDrainReached := time.Since(t.sequentialDrainStart) >= t.sequentialMaxDrain
		t.sequentialDrainMu.Unlock()

		if quietReady {
			t.requestReaderStopOnce()
		}
		if maxDrainReached {
			if !maxDrainEscalated {
				maxDrainEscalated = true
				log.Warnf("Sequential rotation handoff requesting forced shutdown for %q after %s", t.file.Path, t.sequentialMaxDrain)
				t.requestReaderStopOnce()
			}
			forceCloseGrace := sequentialReaderForceCloseGrace
			if t.sequentialForceCloseGrace > 0 {
				forceCloseGrace = t.sequentialForceCloseGrace
			}
			if !t.IsReaderClosed() && time.Since(t.sequentialDrainStart) >= t.sequentialMaxDrain+forceCloseGrace {
				t.forceCloseReader()
			}
		}

		if t.IsReaderClosed() {
			t.finishSequentialDrainWatchdog(maxDrainEscalated)
			return
		}
	}
}

func (t *Tailer) finishSequentialDrainWatchdog(forcedShutdown bool) {
	if forcedShutdown {
		// Max-drain path may leave forward blocked on a full output channel while
		// the reader is already closed; cancel forwarding so the tailer can finish.
		t.requestForwardStopOnce()
	}
}

func (t *Tailer) requestReaderStopOnce() {
	t.readerStopOnce.Do(func() {
		select {
		case t.stop <- struct{}{}:
		default:
		}
	})
}

func (t *Tailer) requestForwardStopOnce() {
	t.forwardCancelOnce.Do(func() {
		t.stopForward()
	})
}
