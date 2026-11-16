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

// errortrackingFlagEvent is something the Remote Flags client can tell the
// handler.
type errortrackingFlagEvent string

const (
	flagTrue      errortrackingFlagEvent = "OnChange(true)"
	flagFalse     errortrackingFlagEvent = "OnChange(false)"
	flagAbsent    errortrackingFlagEvent = "OnNoConfig"
	flagRecovered errortrackingFlagEvent = "SafeRecover"
)

func (e errortrackingFlagEvent) deliver(h pkgremoteflags.FlagHandler) {
	switch e {
	case flagTrue:
		_ = h.OnChange(true)
	case flagFalse:
		_ = h.OnChange(false)
	case flagAbsent:
		h.OnNoConfig()
	case flagRecovered:
		h.SafeRecover(errors.New("unhealthy"), true)
	}
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
	a.errortrackingFlag = newErrortrackingFlagHandler(a.cfgComp, a.logComp, a.setErrortracking, a.isErrortrackingHealthy)
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

// TestErrortrackingFlagHandler_Precedence covers the truth table of the local
// setting against the flag. override_local never reaches the handler: the
// backend does not send it and the handler has no way to read it.
func TestErrortrackingFlagHandler_Precedence(t *testing.T) {
	tests := []struct {
		name   string
		local  string
		events []errortrackingFlagEvent
		want   bool
	}{
		{name: "unset, flag true", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagTrue}, want: true},
		{name: "unset, flag false", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagFalse}, want: false},
		{name: "unset, flag true then false", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagTrue, flagFalse}, want: false},
		{name: "unset, flag true then absent", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagTrue, flagAbsent}, want: false},
		{name: "unset, flag true then recovered", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagTrue, flagRecovered}, want: false},
		{name: "unset, flag true, false, true", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagTrue, flagFalse, flagTrue}, want: true},
		{name: "unset, flag absent from the start", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagAbsent}, want: false},

		{name: "local true, flag true", local: errortrackingLocalTrue, events: []errortrackingFlagEvent{flagTrue}, want: true},
		// A local true is never switched off by the flag, an absent flag or a recovery.
		{name: "local true, flag false", local: errortrackingLocalTrue, events: []errortrackingFlagEvent{flagFalse}, want: true},
		{name: "local true, flag absent", local: errortrackingLocalTrue, events: []errortrackingFlagEvent{flagAbsent}, want: true},
		{name: "local true, flag recovered", local: errortrackingLocalTrue, events: []errortrackingFlagEvent{flagTrue, flagRecovered}, want: true},

		// A local false wins over everything the flag says.
		{name: "local false, flag true", local: errortrackingLocalFalse, events: []errortrackingFlagEvent{flagTrue}, want: false},
		{name: "local false, flag false", local: errortrackingLocalFalse, events: []errortrackingFlagEvent{flagFalse}, want: false},
		{name: "local false, flag absent", local: errortrackingLocalFalse, events: []errortrackingFlagEvent{flagAbsent}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, tt.local)
			var applied []bool
			h := newErrortrackingFlagHandler(cfg, makeLogMock(t), func(on bool) { applied = append(applied, on) }, func() bool { return true })

			for _, e := range tt.events {
				e.deliver(h)
			}

			require.NotEmpty(t, applied)
			assert.Equal(t, tt.want, applied[len(applied)-1])
		})
	}
}

func TestErrortrackingFlagHandler_Identity(t *testing.T) {
	healthy := true
	h := newErrortrackingFlagHandler(configmock.NewFromYAML(t, errortrackingLocalUnset), makeLogMock(t), func(bool) {}, func() bool { return healthy })

	assert.Equal(t, pkgremoteflags.FlagName("agent_errortracking"), h.FlagName())
	assert.True(t, h.IsHealthy())
	healthy = false
	assert.False(t, h.IsHealthy())
}

// The subscriber carries the errortracking handler next to the existing one,
// and the existing one keeps its flag and behavior.
func TestRemoteFlagSubscriber_BothHandlers(t *testing.T) {
	a := getRemoteFlagTestAtel(t, &senderMock{})
	a.errortrackingFlag = newErrortrackingFlagHandler(a.cfgComp, a.logComp, a.setErrortracking, a.isErrortrackingHealthy)

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
	_ = a.errortrackingFlag.OnChange(true)
	require.NotNil(t, testQueue(a))
	assert.Equal(t, 1, countErrortrackingJobs(runner))
	a.SubmitErrorLog(errorLog("while on"))
	a.flushErrortracking(context.Background())
	assert.Len(t, sndr.capturedLogs(), 1)

	// Off: what is buffered is flushed, the queue is released, later
	// submissions are dropped.
	a.SubmitErrorLog(errorLog("buffered when switched off"))
	_ = a.errortrackingFlag.OnChange(false)
	a.errortrackingDrains.Wait()
	assert.Len(t, sndr.capturedLogs(), 2)
	assert.Nil(t, testQueue(a))
	a.SubmitErrorLog(errorLog("after off"))
	a.flushErrortracking(context.Background())
	assert.Len(t, sndr.capturedLogs(), 2)

	// On again: a new queue, and no second flush job.
	_ = a.errortrackingFlag.OnChange(true)
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

	_ = a.errortrackingFlag.OnChange(true)
	_ = a.errortrackingFlag.OnChange(false)
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

func TestErrortrackingFlag_LocalSettingWinsOnTheComponent(t *testing.T) {
	tests := []struct {
		name      string
		local     string
		events    []errortrackingFlagEvent
		wantQueue bool
	}{
		{name: "local true survives flag false", local: errortrackingLocalTrue, events: []errortrackingFlagEvent{flagFalse, flagAbsent, flagRecovered}, wantQueue: true},
		{name: "local false ignores flag true", local: errortrackingLocalFalse, events: []errortrackingFlagEvent{flagTrue}, wantQueue: false},
		{name: "local unset follows flag true", local: errortrackingLocalUnset, events: []errortrackingFlagEvent{flagTrue}, wantQueue: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sndr := &senderMock{}
			a, _ := newErrortrackingFlagTestAtel(t, tt.local, sndr)
			for _, e := range tt.events {
				e.deliver(a.errortrackingFlag)
			}

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

// Agent telemetry being off (or excluded) keeps errortracking off whatever
// the flag says.
func TestSetErrortracking_AgentTelemetryDisabled(t *testing.T) {
	cleanupErrortrackingSlots(t)
	cfg := configmock.NewFromYAML(t, "agent_telemetry:\n  enabled: false\n")
	a := createAtel(cfg, makeLogMock(t), makeTelMock(t), &senderMock{}, &runnerMock{})
	h := newErrortrackingFlagHandler(cfg, makeLogMock(t), a.setErrortracking, a.isErrortrackingHealthy)

	_ = h.OnChange(true)

	assert.Nil(t, testQueue(a))
	assert.False(t, h.IsHealthy())
}

func TestSetErrortracking_AfterStop(t *testing.T) {
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, &senderMock{})
	require.NoError(t, a.stop())

	_ = a.errortrackingFlag.OnChange(true)

	assert.Nil(t, testQueue(a))
}

// Stopping while on still sends what is buffered.
func TestStop_FlushesWhatTheFlagEnabled(t *testing.T) {
	sndr := &senderMock{}
	a, _ := newErrortrackingFlagTestAtel(t, errortrackingLocalUnset, sndr)
	_ = a.errortrackingFlag.OnChange(true)
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
	_ = a.errortrackingFlag.OnChange(true)
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
	a.errortrackingFlag.SafeRecover(nil, true)
	_ = a.errortrackingFlag.OnChange(true)
	assert.True(t, a.errortrackingFlag.IsHealthy())
}

// The troubleshooting flag and the errortracking flag do not affect each other.
func TestErrortrackingFlag_TroubleshootingUnaffected(t *testing.T) {
	sndr := &senderMock{}
	a := getRemoteFlagTestAtel(t, sndr)
	cleanupErrortrackingSlots(t)
	a.errortrackingFlag = newErrortrackingFlagHandler(a.cfgComp, a.logComp, a.setErrortracking, a.isErrortrackingHealthy)

	require.NoError(t, a.flag.OnChange(true))
	assert.Nil(t, testQueue(a), "the troubleshooting flag must not turn errortracking on")
	assert.Contains(t, collectMetricNames(t, a, sndr), "gatedgroup.gatedmetric")

	require.NoError(t, a.errortrackingFlag.OnChange(true))
	assert.NotNil(t, testQueue(a))
	assert.Contains(t, collectMetricNames(t, a, sndr), "gatedgroup.gatedmetric")

	a.errortrackingFlag.OnNoConfig()
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
				_ = a.errortrackingFlag.OnChange(i%2 == 0)
				a.flushErrortracking(context.Background())
			}
		}()
	}
	togglers.Wait()
	close(stop)
	producers.Wait()

	assert.NotPanics(t, func() {
		a.errortrackingFlag.OnNoConfig()
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
	a.setErrortracking(true)
	a.SubmitErrorLog(errorLog("buffered"))

	returned := make(chan struct{})
	go func() {
		a.errortrackingFlag.OnNoConfig()
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
