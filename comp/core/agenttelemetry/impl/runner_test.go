// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenttelemetryimpl

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipRecorder is a cron.Logger counting the runs the scheduler skipped.
type skipRecorder struct {
	skips atomic.Int32
}

func (l *skipRecorder) Info(msg string, _ ...interface{}) {
	if msg == "skip" {
		l.skips.Add(1)
	}
}

func (l *skipRecorder) Error(error, string, ...interface{}) {}

// TestRunnerSkipsRunWhileStillRunning checks that a run still going when its next one is due makes
// that next one skipped instead of running concurrently.
func TestRunnerSkipsRunWhileStillRunning(t *testing.T) {
	logger := &skipRecorder{}
	r := newRunnerImpl().(*runnerImpl)
	r.cronLogger = logger
	r.start()

	var runs atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	// Unblock the first run before stopping: stop waits for running jobs, even when an assertion
	// failed first.
	t.Cleanup(func() {
		unblock()
		<-r.stop().Done()
	})
	r.taskCron.Schedule(&runnerSchedule{schedule: Schedule{StartAfter: 1, Period: 1}}, cron.FuncJob(func() {
		if runs.Add(1) == 1 {
			<-release // the first run lasts past the next scheduled run
		}
	}))

	// The next run comes due while the first one is still running: it is skipped, not started.
	require.Eventually(t, func() bool { return logger.skips.Load() >= 1 }, 5*time.Second, 50*time.Millisecond,
		"no run was skipped while the first one was still running")
	assert.Equal(t, int32(1), runs.Load(), "a run started while the previous one was still running")
	unblock()

	// Once the first run is done, the next scheduled run starts.
	require.Eventually(t, func() bool { return runs.Load() >= 2 }, 5*time.Second, 50*time.Millisecond)
}
