// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	testopms "github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms/testing"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

type fakeExecutor struct {
	prepareCalls atomic.Int32
	stopCalls    atomic.Int32

	mu        sync.Mutex
	executeFn func(ctx context.Context, task *types.Task) (interface{}, error)
	calls     []string
}

func (f *fakeExecutor) Prepare(_ context.Context) error {
	f.prepareCalls.Add(1)
	return nil
}

func (f *fakeExecutor) Execute(ctx context.Context, task *types.Task) (interface{}, error) {
	f.mu.Lock()
	f.calls = append(f.calls, task.Data.ID)
	fn := f.executeFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, task)
	}
	return nil, nil
}

func (f *fakeExecutor) Stop(_ context.Context) error {
	f.stopCalls.Add(1)
	return nil
}

func (f *fakeExecutor) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func newTestConfig(poolSize int32) *config.Config {
	return &config.Config{
		RunnerPoolSize:    poolSize,
		LoopInterval:      time.Millisecond,
		MinBackoff:        time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		WaitBeforeRetry:   time.Millisecond,
		MaxAttempts:       3,
		HeartbeatInterval: 10 * time.Millisecond,
	}
}

func makeTask(id string) *types.Task {
	t := &types.Task{}
	t.Data.Attributes = &types.Attributes{
		JobId:  "job-" + id,
		Client: actionsclientpb.Client_WORKFLOWS,
	}
	t.Data.ID = id
	return t
}

// TestOrchestrator_DispatchesAndPublishesSuccess verifies the orchestrator
// dequeues, dispatches to executor.Execute, and publishes the action
// output via PublishSuccess.
func TestOrchestrator_DispatchesAndPublishesSuccess(t *testing.T) {
	tasks := []*types.Task{makeTask("task-1"), makeTask("task-2")}
	var idx atomic.Int32

	successC := make(chan string, len(tasks))
	opmsClient := &testopms.FakeOpmsClient{
		DequeueTaskFn: func(_ context.Context) (*types.Task, time.Duration, error) {
			i := idx.Add(1) - 1
			if int(i) >= len(tasks) {
				return nil, 0, nil
			}
			return tasks[i], 0, nil
		},
		PublishSuccessFn: func(_ context.Context, _ actionsclientpb.Client, taskID, _, _ string, _ interface{}, _ string) error {
			successC <- taskID
			return nil
		},
	}
	exec := &fakeExecutor{
		executeFn: func(_ context.Context, task *types.Task) (interface{}, error) {
			return "ok-" + task.Data.ID, nil
		},
	}

	o := NewOrchestrator(newTestConfig(2), opmsClient, exec)
	require.NoError(t, o.Start(context.Background()))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = o.Stop(stopCtx)
	})

	got := make([]string, 0, len(tasks))
	for range tasks {
		select {
		case id := <-successC:
			got = append(got, id)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for PublishSuccess, got so far: %v", got)
		}
	}
	assert.ElementsMatch(t, []string{"task-1", "task-2"}, got)
	assert.ElementsMatch(t, []string{"task-1", "task-2"}, exec.recorded())
	assert.EqualValues(t, 1, exec.prepareCalls.Load())
}

// TestOrchestrator_PublishesFailureOnExecuteError verifies that an
// Execute-time error is forwarded to PublishFailure with the right metadata.
func TestOrchestrator_PublishesFailureOnExecuteError(t *testing.T) {
	task := makeTask("task-err")
	var sent atomic.Int32

	failureC := make(chan string, 1)
	opmsClient := &testopms.FakeOpmsClient{
		DequeueTaskFn: func(_ context.Context) (*types.Task, time.Duration, error) {
			if sent.Add(1) > 1 {
				return nil, 0, nil
			}
			return task, 0, nil
		},
		PublishFailureFn: func(_ context.Context, _ actionsclientpb.Client, taskID, _, _ string, _ aperrorpb.ActionPlatformErrorCode, _, _ string) error {
			failureC <- taskID
			return nil
		},
	}
	exec := &fakeExecutor{
		executeFn: func(_ context.Context, _ *types.Task) (interface{}, error) {
			return nil, errors.New("boom")
		},
	}

	o := NewOrchestrator(newTestConfig(1), opmsClient, exec)
	require.NoError(t, o.Start(context.Background()))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = o.Stop(stopCtx)
	})

	select {
	case id := <-failureC:
		assert.Equal(t, "task-err", id)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for PublishFailure")
	}
}

// TestOrchestrator_HeartbeatsDuringExecute verifies the orchestrator emits
// Heartbeat RPCs while Execute is in flight and stops once Execute returns.
func TestOrchestrator_HeartbeatsDuringExecute(t *testing.T) {
	task := makeTask("task-hb")
	var dequeued atomic.Int32
	var heartbeats atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseExecute := func() { releaseOnce.Do(func() { close(release) }) }

	opmsClient := &testopms.FakeOpmsClient{
		DequeueTaskFn: func(_ context.Context) (*types.Task, time.Duration, error) {
			if dequeued.Add(1) > 1 {
				return nil, 0, nil
			}
			return task, 0, nil
		},
		HeartbeatFn: func(_ context.Context, _ actionsclientpb.Client, _, _, _ string) error {
			heartbeats.Add(1)
			return nil
		},
	}
	exec := &fakeExecutor{
		executeFn: func(ctx context.Context, _ *types.Task) (interface{}, error) {
			select {
			case <-release:
			case <-ctx.Done():
			}
			return "done", nil
		},
	}

	o := NewOrchestrator(newTestConfig(1), opmsClient, exec)
	require.NoError(t, o.Start(context.Background()))
	t.Cleanup(func() {
		releaseExecute()
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = o.Stop(stopCtx)
	})

	require.Eventually(t, func() bool { return heartbeats.Load() >= 2 }, time.Second, 5*time.Millisecond,
		"expected at least two heartbeats while Execute blocks")
	releaseExecute()

	// After Execute returns no more heartbeats fire. Give the cancel a
	// moment to propagate, then assert quiescence.
	time.Sleep(40 * time.Millisecond)
	settled := heartbeats.Load()
	time.Sleep(40 * time.Millisecond)
	assert.Equal(t, settled, heartbeats.Load(), "heartbeats must stop once Execute returns")
}

// TestOrchestrator_DoesNotDispatchWhenDequeueReturnsNil verifies the
// polling loop just backs off and does not invoke the executor.
func TestOrchestrator_DoesNotDispatchWhenDequeueReturnsNil(t *testing.T) {
	var calls atomic.Int32
	opmsClient := &testopms.FakeOpmsClient{
		DequeueTaskFn: func(_ context.Context) (*types.Task, time.Duration, error) {
			calls.Add(1)
			return nil, 0, nil
		},
	}
	exec := &fakeExecutor{}

	o := NewOrchestrator(newTestConfig(1), opmsClient, exec)
	require.NoError(t, o.Start(context.Background()))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = o.Stop(stopCtx)
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && calls.Load() < 3 {
		time.Sleep(5 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, calls.Load(), int32(3))
	assert.Empty(t, exec.recorded())
}

// TestOrchestrator_StopCallsExecutorStop verifies Stop forwards to the
// executor after the polling loop has exited.
func TestOrchestrator_StopCallsExecutorStop(t *testing.T) {
	exec := &fakeExecutor{}
	o := NewOrchestrator(newTestConfig(1), &testopms.FakeOpmsClient{}, exec)
	require.NoError(t, o.Start(context.Background()))

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, o.Stop(stopCtx))
	assert.EqualValues(t, 1, exec.stopCalls.Load())
}

// TestOrchestrator_StopBeforeStartIsSafe ensures Stop is a no-op when
// Start was never called.
func TestOrchestrator_StopBeforeStartIsSafe(t *testing.T) {
	o := NewOrchestrator(newTestConfig(1), &testopms.FakeOpmsClient{}, &fakeExecutor{})
	stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	assert.NoError(t, o.Stop(stopCtx))
}

// Compile-time check that the fake still satisfies opms.Client.
var _ opms.Client = (*testopms.FakeOpmsClient)(nil)
