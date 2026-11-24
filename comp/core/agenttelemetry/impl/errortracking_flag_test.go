// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

const (
	errortrackingLocalUnset = "agent_telemetry:\n  enabled: true\n"
	errortrackingLocalTrue  = "agent_telemetry:\n  enabled: true\n  errortracking:\n    enabled: true\n"
	errortrackingLocalFalse = "agent_telemetry:\n  enabled: true\n  errortracking:\n    enabled: false\n"
)

// setErrortrackingKey delivers the flag value to the handler, as the Remote
// Flags client does.
func setErrortrackingKey(a *atel, on bool) {
	_ = a.errortrackingFlag.OnChange(pkgremoteflags.FlagValue(on))
}

// unsetErrortrackingKey tells the handler the flag is absent, as the Remote
// Flags client does when the flag is removed.
func unsetErrortrackingKey(a *atel) {
	a.errortrackingFlag.OnNoConfig()
}

// cleanupErrortrackingSlots clears the package-global logger slots a toggle
// registers.
func cleanupErrortrackingSlots(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		pkglogsetup.RegisterErrortrackingSubmitter(nil)
		pkglogsetup.RegisterErrortrackingBouncer(nil)
	})
}

// stoppableRunner is a runnerMock whose stop reports every job finished, so
// atel.stop does not wait forever on the mock's never-done context.
type stoppableRunner struct{ *runnerMock }

func (r stoppableRunner) stop() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// newErrortrackingFlagTestAtel builds a started atel over the given local
// configuration with the errortracking flag handler attached the way
// NewComponent does.
func newErrortrackingFlagTestAtel(t *testing.T, yaml string, sndr sender) (*atel, *runnerMock) {
	t.Helper()
	cleanupErrortrackingSlots(t)

	runner := &runnerMock{}
	a := getTestAtel(t, nil, yaml, sndr, nil, stoppableRunner{runner})
	require.True(t, a.enabled)
	a.errortrackingFlag = newErrortrackingFlagHandler(a.setErrortracking, a.errortrackingLocallyDisabled, a.isErrortrackingHealthy)
	require.NoError(t, a.start())
	t.Cleanup(a.cancel)
	return a, runner
}

func countErrortrackingJobs(runner *runnerMock) int {
	n := 0
	for _, j := range runner.jobs {
		if j.profiles == nil {
			n++
		}
	}
	return n
}

// The handler names its flag and shares the component's health probe.
func TestErrortrackingFlagHandler_NameAndHealth(t *testing.T) {
	healthy := true
	h := newErrortrackingFlagHandler(func(bool) bool { return false }, func() bool { return false }, func() bool { return healthy })

	assert.Equal(t, pkgremoteflags.FlagName("agent_errortracking"), h.FlagName())
	assert.True(t, h.IsHealthy())
	healthy = false
	assert.False(t, h.IsHealthy())
}

// The subscriber carries the errortracking handler next to the existing one,
// and the existing one keeps its flag and behavior.
func TestRemoteFlagSubscriber_BothHandlers(t *testing.T) {
	a := getRemoteFlagTestAtel(t, &senderMock{})
	a.errortrackingFlag = newErrortrackingFlagHandler(a.setErrortracking, a.errortrackingLocallyDisabled, a.isErrortrackingHealthy)

	handlers := remoteFlagSubscriber{a.flag, a.errortrackingFlag}.Handlers()

	names := make([]pkgremoteflags.FlagName, 0, len(handlers))
	for _, h := range handlers {
		names = append(names, h.FlagName())
	}
	assert.Equal(t, []pkgremoteflags.FlagName{flagTroubleshooting, flagErrortracking}, names)
}

func TestSetErrortracking_Toggle(t *testing.T) {
	sndr := &senderMock{}
	a, runner := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, sndr)

	// Off by default: no queue, no flush job, submissions are dropped.
	assert.Nil(t, testQueue(a))
	assert.Zero(t, countErrortrackingJobs(runner))
	a.SubmitErrorLog(errorLog("before the flag"))
	a.flushErrortracking(context.Background())
	assert.Empty(t, sndr.capturedLogs())

	// On: the queue exists, the flush job is scheduled once.
	setErrortrackingKey(a, true)
	require.NotNil(t, testQueue(a))
	assert.Equal(t, 1, countErrortrackingJobs(runner))
	a.SubmitErrorLog(errorLog("while on"))
	a.flushErrortracking(context.Background())
	assert.Len(t, sndr.capturedLogs(), 1)

	// Off: what is buffered is flushed, the queue is released, later
	// submissions are dropped.
	a.SubmitErrorLog(errorLog("buffered when switched off"))
	setErrortrackingKey(a, false)
	a.errortrackingDrains.Wait()
	assert.Len(t, sndr.capturedLogs(), 2)
	assert.Nil(t, testQueue(a))
	a.SubmitErrorLog(errorLog("after off"))
	a.flushErrortracking(context.Background())
	assert.Len(t, sndr.capturedLogs(), 2)

	// On again: a new queue, and no second flush job.
	setErrortrackingKey(a, true)
	require.NotNil(t, testQueue(a))
	assert.Equal(t, 1, countErrortrackingJobs(runner))
	a.SubmitErrorLog(errorLog("second time on"))
	a.flushErrortracking(context.Background())
	assert.Len(t, sndr.capturedLogs(), 3)
}

// The flush job scheduled by the first activation keeps running through the
// runner; once off it finds no queue and does nothing.
func TestSetErrortracking_FlushJobIdlesWhileOff(t *testing.T) {
	sndr := &senderMock{}
	a, runner := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, sndr)

	setErrortrackingKey(a, true)
	setErrortrackingKey(a, false)
	require.Equal(t, 1, countErrortrackingJobs(runner))

	callsBefore := sndr.sendLogsCalls()
	findErrortrackingJob(t, runner).Run()
	assert.Equal(t, callsBefore, sndr.sendLogsCalls())
}

// Activation before the runner starts schedules the job at start.
func TestSetErrortracking_BeforeStart(t *testing.T) {
	cleanupErrortrackingSlots(t)
	runner := &runnerMock{}
	a := getTestAtel(t, nil, errortrackingLocalUnset, &senderMock{}, nil, runner)

	a.setErrortracking(true)
	require.NotNil(t, testQueue(a))
	assert.Zero(t, countErrortrackingJobs(runner))

	require.NoError(t, a.start())
	t.Cleanup(a.cancel)
	assert.Equal(t, 1, countErrortrackingJobs(runner))
}

// Precedence between the flag and the local setting: an explicit local false
// always wins over the flag; a local true stays on whatever the flag does;
// with the key unset the flag decides, and a false, removed or recovered flag
// switches off only what the flag turned on.
func TestErrortracking_FlagAndLocalPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		local     string
		change    func(a *atel)
		wantQueue bool
	}{
		{name: "unset, flag true", local: errortrackingLocalUnset, change: func(a *atel) { setErrortrackingKey(a, true) }, wantQueue: true},
		{name: "unset, flag false", local: errortrackingLocalUnset, change: func(a *atel) { setErrortrackingKey(a, false) }, wantQueue: false},
		{name: "unset, flag true then false", local: errortrackingLocalUnset, change: func(a *atel) { setErrortrackingKey(a, true); setErrortrackingKey(a, false) }, wantQueue: false},
		{name: "unset, flag true then removed", local: errortrackingLocalUnset, change: func(a *atel) { setErrortrackingKey(a, true); unsetErrortrackingKey(a) }, wantQueue: false},
		{name: "unset, flag true then recovered", local: errortrackingLocalUnset, change: func(a *atel) {
			setErrortrackingKey(a, true)
			a.errortrackingFlag.SafeRecover(errors.New("unhealthy"), true)
		}, wantQueue: false},
		{name: "local false, flag true", local: errortrackingLocalFalse, change: func(a *atel) { setErrortrackingKey(a, true) }, wantQueue: false},
		{name: "local true, flag absent", local: errortrackingLocalTrue, change: func(a *atel) { unsetErrortrackingKey(a) }, wantQueue: true},
		{name: "local true, flag false", local: errortrackingLocalTrue, change: func(a *atel) { setErrortrackingKey(a, false) }, wantQueue: true},
		{name: "local true, flag true then false", local: errortrackingLocalTrue, change: func(a *atel) { setErrortrackingKey(a, true); setErrortrackingKey(a, false) }, wantQueue: true},
		{name: "local true, flag true then recovered", local: errortrackingLocalTrue, change: func(a *atel) {
			setErrortrackingKey(a, true)
			a.errortrackingFlag.SafeRecover(errors.New("unhealthy"), true)
		}, wantQueue: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sndr := &senderMock{}
			a, _ := newErrortrackingFlagTestAtel(t, tt.local, sndr)

			tt.change(a)
			a.errortrackingDrains.Wait()

			assert.Equal(t, tt.wantQueue, testQueue(a) != nil)
			a.SubmitErrorLog(errorLog("x"))
			a.flushErrortracking(context.Background())
			if tt.wantQueue {
				assert.Len(t, sndr.capturedLogs(), 1)
			} else {
				assert.Empty(t, sndr.capturedLogs())
			}
		})
	}
}

// A flag that is true while the local setting is already on does not claim the
// feature: the local setting stays in charge and nothing is rescheduled.
func TestErrortracking_LocalOnIsNotClaimedByTheFlag(t *testing.T) {
	a, runner := newErrortrackingFlagTestAtel(t, errortrackingLocalTrue, &senderMock{})
	require.Equal(t, 1, countErrortrackingJobs(runner))

	setErrortrackingKey(a, true)
	setErrortrackingKey(a, false)
	unsetErrortrackingKey(a)

	assert.NotNil(t, testQueue(a))
	assert.Equal(t, 1, countErrortrackingJobs(runner))
}

// Agent telemetry being off (or excluded) keeps errortracking off whatever
// the flag says, and the feature reports unhealthy so the client recovers the
// flag.
func TestErrortracking_AgentTelemetryDisabled(t *testing.T) {
	cleanupErrortrackingSlots(t)
	cfg := configmock.NewFromYAML(t, "agent_telemetry:\n  enabled: false\n")
	a := createAtel(cfg, makeLogMock(t), makeTelMock(t), &senderMock{}, &runnerMock{})
	a.errortrackingFlag = newErrortrackingFlagHandler(a.setErrortracking, a.errortrackingLocallyDisabled, a.isErrortrackingHealthy)

	setErrortrackingKey(a, true)

	assert.Nil(t, testQueue(a))
	assert.False(t, a.errortrackingFlag.IsHealthy())
}

func TestSetErrortracking_AfterStop(t *testing.T) {
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, &senderMock{})
	require.NoError(t, a.stop())

	setErrortrackingKey(a, true)

	assert.Nil(t, testQueue(a))
}

// Stopping while on still sends what is buffered.
func TestStop_FlushesWhatTheFlagEnabled(t *testing.T) {
	sndr := &senderMock{}
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, sndr)
	setErrortrackingKey(a, true)
	a.SubmitErrorLog(errorLog("last words"))

	require.NoError(t, a.stop())

	assert.Len(t, sndr.capturedLogs(), 1)
}

// logFailingSender fails the errortracking sends only.
type logFailingSender struct {
	*senderMock
	fail atomic.Bool
}

func (s *logFailingSender) sendLogsBatch(ctx context.Context, logs []Log) error {
	if s.fail.Load() {
		return errors.New("intake unavailable")
	}
	return s.senderMock.sendLogsBatch(ctx, logs)
}

func (s *logFailingSender) flushSession(_ *senderSession) error {
	return nil
}

// One feature's flush failures never make the other one unhealthy.
func TestErrortrackingHealthIsIndependent(t *testing.T) {
	sndr := &logFailingSender{senderMock: &senderMock{}}
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, sndr)
	a.flag = newRemoteFlagHandler(flagTroubleshooting, a.isHealthy)
	setErrortrackingKey(a, true)
	require.True(t, a.errortrackingFlag.IsHealthy())

	sndr.fail.Store(true)
	for i := 0; i < maxConsecutiveFlushFailures; i++ {
		assert.True(t, a.errortrackingFlag.IsHealthy(), "still healthy after %d failures", i)
		a.SubmitErrorLog(errorLog("x"))
		a.flushErrortracking(context.Background())
	}
	assert.False(t, a.errortrackingFlag.IsHealthy())
	assert.True(t, a.flag.IsHealthy(), "troubleshooting health must not follow errortracking flushes")

	// A success clears the streak.
	sndr.fail.Store(false)
	a.SubmitErrorLog(errorLog("x"))
	a.flushErrortracking(context.Background())
	assert.True(t, a.errortrackingFlag.IsHealthy())

	// Switching on again starts from a clean signal.
	sndr.fail.Store(true)
	for i := 0; i < maxConsecutiveFlushFailures; i++ {
		a.SubmitErrorLog(errorLog("x"))
		a.flushErrortracking(context.Background())
	}
	require.False(t, a.errortrackingFlag.IsHealthy())
	unsetErrortrackingKey(a)
	setErrortrackingKey(a, true)
	assert.True(t, a.errortrackingFlag.IsHealthy())
}

// The troubleshooting flag and the errortracking flag do not affect each other.
func TestErrortrackingFlag_TroubleshootingUnaffected(t *testing.T) {
	sndr := &senderMock{}
	a := getRemoteFlagTestAtel(t, sndr)
	cleanupErrortrackingSlots(t)
	a.errortrackingFlag = newErrortrackingFlagHandler(a.setErrortracking, a.errortrackingLocallyDisabled, a.isErrortrackingHealthy)

	require.NoError(t, a.flag.OnChange(true))
	assert.Nil(t, testQueue(a), "the troubleshooting flag must not turn errortracking on")
	assert.Contains(t, collectMetricNames(t, a, sndr), "gatedgroup.gatedmetric")

	setErrortrackingKey(a, true)
	assert.NotNil(t, testQueue(a))
	assert.Contains(t, collectMetricNames(t, a, sndr), "gatedgroup.gatedmetric")

	unsetErrortrackingKey(a)
	assert.Nil(t, testQueue(a))
	assert.True(t, a.flag.enabled.Load(), "the troubleshooting flag keeps its own state")
}

// Switching on and off while producers submit must be race free, never block
// the producers and never panic.
func TestSetErrortracking_ConcurrentToggleAndSubmit(t *testing.T) {
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, &senderMock{})

	var producers sync.WaitGroup
	stop := make(chan struct{})
	for p := 0; p < 4; p++ {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					a.SubmitErrorLog(errortracking.ErrorLog{})
				}
			}
		}()
	}

	var togglers sync.WaitGroup
	for g := 0; g < 3; g++ {
		togglers.Add(1)
		go func() {
			defer togglers.Done()
			for i := 0; i < 50; i++ {
				setErrortrackingKey(a, i%2 == 0)
				a.flushErrortracking(context.Background())
			}
		}()
	}
	togglers.Wait()
	close(stop)
	producers.Wait()

	a.errortrackingDrains.Wait()

	assert.NotPanics(t, func() {
		unsetErrortrackingKey(a)
		require.NoError(t, a.stop())
	})
	assert.Nil(t, testQueue(a))
}

// gatedSender blocks errortracking sends until released, then returns err.
type gatedSender struct {
	*senderMock
	entered chan struct{}
	release chan struct{}
	err     error
}

func newGatedSender(err error) *gatedSender {
	return &gatedSender{senderMock: &senderMock{}, entered: make(chan struct{}, 8), release: make(chan struct{}), err: err}
}

func (s *gatedSender) sendLogsBatch(ctx context.Context, logs []Log) error {
	s.entered <- struct{}{}
	<-s.release
	if s.err != nil {
		return s.err
	}
	return s.senderMock.sendLogsBatch(ctx, logs)
}

// Switching off must return while the release of the queue is still stuck
// on a slow intake, because the Remote Flags client calls it under its lock.
// Stop waits for that release.
func TestSetErrortracking_OffDoesNotWaitForTheFlush(t *testing.T) {
	sndr := newGatedSender(nil)
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, sndr)
	setErrortrackingKey(a, true)
	a.SubmitErrorLog(errorLog("buffered"))

	returned := make(chan struct{})
	go func() {
		unsetErrortrackingKey(a)
		close(returned)
	}()
	<-sndr.entered // the release of the queue is now blocked in the sender
	<-returned
	assert.Nil(t, testQueue(a))

	stopped := make(chan struct{})
	go func() {
		_ = a.stop()
		close(stopped)
	}()
	close(sndr.release)
	<-stopped
	assert.Len(t, sndr.capturedLogs(), 1, "stop must wait for the release of the queue")
}

// A release that outlives its queue must not feed the health signal of the
// queue that replaced it.
func TestSetErrortracking_StaleFlushDoesNotCountAsFailure(t *testing.T) {
	sndr := newGatedSender(errors.New("intake unavailable"))
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, sndr)
	a.setErrortracking(true)
	a.SubmitErrorLog(errorLog("buffered"))
	a.setErrortracking(false)
	<-sndr.entered
	a.setErrortracking(true)

	close(sndr.release)
	a.errortrackingDrains.Wait()

	assert.True(t, a.isErrortrackingHealthy())
	assert.Zero(t, a.errLogsFlushFailures.Load())
}
