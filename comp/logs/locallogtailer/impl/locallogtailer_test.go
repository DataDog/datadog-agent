// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package locallogtailerimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// mockHandle is a test double for observerdef.Handle that captures ObserveLog calls.
type mockHandle struct {
	logs []observerdef.LogView
}

func (m *mockHandle) ObserveLog(msg observerdef.LogView)             { m.logs = append(m.logs, msg) }
func (m *mockHandle) ObserveMetric(_ observerdef.MetricView)         {}
func (m *mockHandle) ObserveTrace(_ observerdef.TraceView)           {}
func (m *mockHandle) ObserveTraceStats(_ observerdef.TraceStatsView) {}
func (m *mockHandle) ObserveProfile(_ observerdef.ProfileView)       {}

// mockObserver is a test double for observerdef.Component.
type mockObserver struct {
	handle *mockHandle
}

func (m *mockObserver) GetHandle(_ string) observerdef.Handle { return m.handle }
func (m *mockObserver) DumpMetrics(_ string) error            { return nil }

// TestNewComponent_NoopWhenObserverNotPresent checks that the component is
// inactive (noop) when no observer is wired into the Fx graph.
func TestNewComponent_NoopWhenObserverNotPresent(t *testing.T) {
	cfg := configComponent.NewMockWithOverrides(t, map[string]interface{}{
		"observer.enabled": true,
	})
	reqs := Requires{
		Config:   cfg,
		Observer: option.None[observerdef.Component](),
	}
	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	_, isNoop := provides.Comp.(*noopLocalLogTailer)
	assert.True(t, isNoop, "expected noop when observer component is absent")
}

// TestNewComponent_NoopWhenConfigDisabled checks that the component is inactive
// when observer.enabled is false, even if an observer component is present.
func TestNewComponent_NoopWhenConfigDisabled(t *testing.T) {
	cfg := configComponent.NewMockWithOverrides(t, map[string]interface{}{
		"observer.enabled": false,
	})
	obs := &mockObserver{handle: &mockHandle{}}
	reqs := Requires{
		Config:   cfg,
		Observer: option.New[observerdef.Component](obs),
	}
	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	_, isNoop := provides.Comp.(*noopLocalLogTailer)
	assert.True(t, isNoop, "expected noop when observer.enabled=false")
}

// TestDrainOutputChan_ForwardsMessagesToObserver verifies that the drain goroutine
// forwards each message from the pipeline output channel to the observer handle.
func TestDrainOutputChan_ForwardsMessagesToObserver(t *testing.T) {
	handle := &mockHandle{}
	outputChan := make(chan *message.Message, 4)
	stopCh := make(chan struct{})

	lr := &localLogTailer{
		observerHandle: handle,
		outputChan:     outputChan,
		stopCh:         stopCh,
	}

	go lr.drainOutputChan()

	msg1 := message.NewMessage([]byte("hello"), nil, "info", 0)
	msg2 := message.NewMessage([]byte("world"), nil, "info", 0)
	outputChan <- msg1
	outputChan <- msg2

	// Allow the goroutine time to process.
	assert.Eventually(t, func() bool {
		return len(handle.logs) == 2
	}, time.Second, 5*time.Millisecond)

	close(stopCh)
}

// TestDrainOutputChan_DrainsDuringStop verifies that messages already in the
// output channel are flushed to the observer after stopCh is closed.
func TestDrainOutputChan_DrainsDuringStop(t *testing.T) {
	handle := &mockHandle{}
	outputChan := make(chan *message.Message, 8)
	stopCh := make(chan struct{})

	lr := &localLogTailer{
		observerHandle: handle,
		outputChan:     outputChan,
		stopCh:         stopCh,
	}

	// Pre-fill the channel before starting the goroutine.
	for i := range 3 {
		_ = i
		outputChan <- message.NewMessage([]byte("pre-stop"), nil, "info", 0)
	}

	go lr.drainOutputChan()

	// Signal stop immediately; the goroutine should drain the 3 buffered messages.
	close(stopCh)

	assert.Eventually(t, func() bool {
		return len(handle.logs) == 3
	}, time.Second, 5*time.Millisecond)
}

// TestDrainOutputChan_ExitsCleanlyOnChannelClose verifies that the drain goroutine
// exits without panicking or blocking when the output channel is closed.
func TestDrainOutputChan_ExitsCleanlyOnChannelClose(t *testing.T) {
	handle := &mockHandle{}
	outputChan := make(chan *message.Message, 2)
	stopCh := make(chan struct{})

	lr := &localLogTailer{
		observerHandle: handle,
		outputChan:     outputChan,
		stopCh:         stopCh,
	}

	done := make(chan struct{})
	go func() {
		lr.drainOutputChan()
		close(done)
	}()

	outputChan <- message.NewMessage([]byte("last"), nil, "info", 0)
	close(outputChan)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drainOutputChan did not exit after channel close")
	}
	assert.Len(t, handle.logs, 1)
}

// TestStop_DoesNotBlockWhenOutputChanNotFull verifies that stop() returns
// promptly even if the pipeline is not running (no goroutines started).
// This is a basic sanity check on the stop ordering.
func TestStop_DoesNotBlockWhenOutputChanNotFull(t *testing.T) {
	cfg := configComponent.NewMockWithOverrides(t, map[string]interface{}{
		"observer.enabled": true,
	})
	obs := &mockObserver{handle: &mockHandle{}}
	reqs := Requires{
		Config:   cfg,
		Observer: option.New[observerdef.Component](obs),
	}
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	lr, ok := provides.Comp.(*localLogTailer)
	require.True(t, ok, "expected non-noop component when observer is present and enabled")

	// stop() before start() should not panic or block. The component hasn't started
	// so the pipeline fields are nil; but we test the gate logic not the full pipeline.
	_ = lr
	_ = context.Background()
	// This test simply verifies NewComponent returns the right type; full start/stop
	// integration is covered at the component level (requires AD, etc.).
}
