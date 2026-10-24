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
	"time"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type submissionSender struct {
	senderMock
	send func(context.Context, []agenttelemetry.Log) error
}

func (s *submissionSender) sendLogsBatch(ctx context.Context, logs []agenttelemetry.Log) error {
	return s.send(ctx, logs)
}

type submissionRunner struct{ runnerMock }

func (*submissionRunner) stop() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func startSubmissionTelemetry(t *testing.T, s sender, capacity int) (*atel, func() error) {
	t.Helper()
	a := &atel{
		enabled:              true,
		atelCfg:              &Config{},
		logComp:              logmock.New(t),
		sender:               s,
		runner:               &submissionRunner{},
		logsCh:               make(chan agenttelemetry.Log, capacity),
		shutdownDrainTimeout: time.Second,
	}
	require.NoError(t, a.start())
	var once sync.Once
	var err error
	stop := func() error {
		once.Do(func() { err = a.stop() })
		return err
	}
	t.Cleanup(func() { require.NoError(t, stop()) })
	return a, stop
}

func TestSubmitLogInactive(t *testing.T) {
	assert.False(t, (&atel{}).SubmitLog(agenttelemetry.Log{}))
	assert.False(t, (&atel{enabled: true, logsCh: make(chan agenttelemetry.Log, 1)}).SubmitLog(agenttelemetry.Log{}))
}

func TestSubmitLogDeliversAsynchronously(t *testing.T) {
	sent := make(chan []agenttelemetry.Log, 1)
	s := &submissionSender{send: func(_ context.Context, logs []agenttelemetry.Log) error {
		sent <- logs
		return nil
	}}
	a, _ := startSubmissionTelemetry(t, s, 2)
	log := agenttelemetry.Log{Message: "crash", Level: agenttelemetry.LogLevelError, Count: 1, ErrorKind: "ddinjector_crash"}
	require.True(t, a.SubmitLog(log))
	assert.Equal(t, []agenttelemetry.Log{log}, <-sent)
}

func TestSubmitLogDropsOnOverflowAndDrainsOnShutdown(t *testing.T) {
	started := make(chan struct{})
	drained := make(chan []agenttelemetry.Log, 1)
	calls := 0 // Only the worker, then the shutdown drain, invokes the sender.
	s := &submissionSender{send: func(ctx context.Context, logs []agenttelemetry.Log) error {
		calls++
		if calls == 1 {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		drained <- logs
		return nil
	}}
	a, stop := startSubmissionTelemetry(t, s, 2)
	require.True(t, a.SubmitLog(agenttelemetry.Log{Message: "in flight"}))
	<-started
	// A blocked intake must not block the producer. The queue remains bounded.
	require.True(t, a.SubmitLog(agenttelemetry.Log{Message: "queued 1"}))
	require.True(t, a.SubmitLog(agenttelemetry.Log{Message: "queued 2"}))
	assert.False(t, a.SubmitLog(agenttelemetry.Log{Message: "overflow"}))
	require.NoError(t, stop())
	assert.Equal(t, []agenttelemetry.Log{{Message: "queued 1"}, {Message: "queued 2"}}, <-drained)
	assert.False(t, a.SubmitLog(agenttelemetry.Log{Message: "after stop"}))
}

func TestSubmitLogConcurrentShutdown(t *testing.T) {
	a, stop := startSubmissionTelemetry(t, &senderMock{}, 2)
	done := make(chan struct{})
	started := make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		for {
			a.SubmitLog(agenttelemetry.Log{Message: "concurrent"})
			select {
			case <-a.cancelCtx.Done():
				return
			default:
			}
		}
	}()
	<-started
	require.NoError(t, stop())
	<-done
	assert.False(t, a.SubmitLog(agenttelemetry.Log{}))
}

func TestSubmitLogSendFailureKeepsWorkerRunning(t *testing.T) {
	attempted := make(chan string, 1)
	s := &submissionSender{send: func(_ context.Context, logs []agenttelemetry.Log) error {
		attempted <- logs[0].Message
		return errors.New("intake unavailable")
	}}
	a, _ := startSubmissionTelemetry(t, s, 2)
	require.True(t, a.SubmitLog(agenttelemetry.Log{Message: "first"}))
	assert.Equal(t, "first", <-attempted)
	require.True(t, a.SubmitLog(agenttelemetry.Log{Message: "second"}))
	assert.Equal(t, "second", <-attempted)
}

func TestSubmitLogShutdownDrainDeadline(t *testing.T) {
	started := make(chan struct{})
	drained := make(chan error, 1)
	calls := 0
	s := &submissionSender{send: func(ctx context.Context, _ []agenttelemetry.Log) error {
		calls++
		if calls == 1 {
			close(started)
			<-ctx.Done()
		} else {
			drained <- ctx.Err()
		}
		return ctx.Err()
	}}
	a, stop := startSubmissionTelemetry(t, s, 2)
	// An exhausted shutdown budget is deterministic and needs no timer wait.
	a.shutdownDrainTimeout = 0
	require.True(t, a.SubmitLog(agenttelemetry.Log{Message: "in flight"}))
	<-started
	require.True(t, a.SubmitLog(agenttelemetry.Log{Message: "queued"}))
	require.NoError(t, stop())
	assert.ErrorIs(t, <-drained, context.DeadlineExceeded)
}
