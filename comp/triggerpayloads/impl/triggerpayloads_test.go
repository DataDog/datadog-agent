// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package triggerpayloadsimpl

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	egressmock "github.com/DataDog/datadog-agent/comp/healthplatform/egress/mock"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	triggerpayloads "github.com/DataDog/datadog-agent/comp/triggerpayloads/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type fakePayload struct {
	calls atomic.Int32
	err   error
}

func (f *fakePayload) send(context.Context) error {
	f.calls.Add(1)
	return f.err
}

type fakeInventoryAgent struct {
	fakePayload
}

func (f *fakeInventoryAgent) Set(string, interface{})     {}
func (f *fakeInventoryAgent) Get() map[string]interface{} { return nil }
func (f *fakeInventoryAgent) SendNow() error              { return f.send(context.Background()) }

type fakeInventoryHost struct {
	fakePayload
}

func (f *fakeInventoryHost) Refresh()       {}
func (f *fakeInventoryHost) SendNow() error { return f.send(context.Background()) }

type fakeInventoryChecks struct {
	fakePayload
}

func (f *fakeInventoryChecks) Set(string, string, interface{})                   {}
func (f *fakeInventoryChecks) GetInstanceMetadata(string) map[string]interface{} { return nil }
func (f *fakeInventoryChecks) Refresh()                                          {}
func (f *fakeInventoryChecks) SendNow() error                                    { return f.send(context.Background()) }

func newTestTriggerPayloads(t *testing.T, inventory, health *fakePayload) *triggerPayloads {
	return &triggerPayloads{
		log: logmock.New(t),
		payloads: map[string]sendFunc{
			triggerpayloads.PayloadInventoryAgent: inventory.send,
			triggerpayloads.PayloadAgentHealth:    health.send,
		},
	}
}

func parseTask(t *testing.T, raw string) rcclienttypes.AgentTaskConfig {
	task, err := rcclienttypes.ParseConfigAgentTask([]byte(raw), state.Metadata{})
	require.NoError(t, err)
	return task
}

func TestNewComponent(t *testing.T) {
	inventoryAgent, inventoryHost, inventoryChecks := &fakeInventoryAgent{}, &fakeInventoryHost{}, &fakeInventoryChecks{}
	provides := NewComponent(Requires{
		Log:             logmock.New(t),
		InventoryAgent:  inventoryAgent,
		InventoryHost:   inventoryHost,
		InventoryChecks: inventoryChecks,
		HealthEgress:    egressmock.New(),
	})

	require.NotNil(t, provides.Comp)
	require.NotNil(t, provides.RCListener.Listener)

	processed, err := provides.RCListener.Listener(rcclienttypes.TaskTriggerPayloads, parseTask(t, `{"task_type":"trigger_payloads","uuid":"a"}`))
	assert.True(t, processed)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), inventoryAgent.calls.Load())
	assert.Equal(t, int32(1), inventoryHost.calls.Load())
	assert.Equal(t, int32(1), inventoryChecks.calls.Load())

	processed, err = provides.RCListener.Listener(rcclienttypes.TaskTriggerPayloads, parseTask(t, `{"task_type":"trigger_payloads","uuid":"b","args":{"payloads":"inventory-host,inventory-checks"}}`))
	assert.True(t, processed)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), inventoryAgent.calls.Load())
	assert.Equal(t, int32(2), inventoryHost.calls.Load())
	assert.Equal(t, int32(2), inventoryChecks.calls.Load())
}

func TestHandleAgentTaskOtherType(t *testing.T) {
	inventory, health := &fakePayload{}, &fakePayload{}
	tp := newTestTriggerPayloads(t, inventory, health)

	processed, err := tp.handleAgentTask(rcclienttypes.TaskFlare, parseTask(t, `{"task_type":"flare","uuid":"a"}`))
	assert.False(t, processed)
	assert.NoError(t, err)
	assert.Zero(t, inventory.calls.Load())
	assert.Zero(t, health.calls.Load())
}

func TestHandleAgentTask(t *testing.T) {
	tests := []struct {
		name              string
		task              string
		expectedInventory int32
		expectedHealth    int32
	}{
		{
			name:              "no args",
			task:              `{"task_type":"trigger_payloads","uuid":"a"}`,
			expectedInventory: 1,
			expectedHealth:    1,
		},
		{
			name:              "null payloads",
			task:              `{"task_type":"trigger_payloads","uuid":"a","args":{"payloads":null}}`,
			expectedInventory: 1,
			expectedHealth:    1,
		},
		{
			name:              "empty payloads",
			task:              `{"task_type":"trigger_payloads","uuid":"a","args":{"payloads":""}}`,
			expectedInventory: 1,
			expectedHealth:    1,
		},
		{
			name:              "subset",
			task:              `{"task_type":"trigger_payloads","uuid":"a","args":{"payloads":"agent-health"}}`,
			expectedInventory: 0,
			expectedHealth:    1,
		},
		{
			name:              "spaces and empty entries",
			task:              `{"task_type":"trigger_payloads","uuid":"a","args":{"payloads":" agent-health , ,inventory-agent,"}}`,
			expectedInventory: 1,
			expectedHealth:    1,
		},
		{
			name:              "only separators",
			task:              `{"task_type":"trigger_payloads","uuid":"a","args":{"payloads":" , "}}`,
			expectedInventory: 1,
			expectedHealth:    1,
		},
		{
			name:              "duplicates",
			task:              `{"task_type":"trigger_payloads","uuid":"a","args":{"payloads":"inventory-agent,inventory-agent"}}`,
			expectedInventory: 1,
			expectedHealth:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inventory, health := &fakePayload{}, &fakePayload{}
			tp := newTestTriggerPayloads(t, inventory, health)

			processed, err := tp.handleAgentTask(rcclienttypes.TaskTriggerPayloads, parseTask(t, tt.task))
			assert.True(t, processed)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedInventory, inventory.calls.Load())
			assert.Equal(t, tt.expectedHealth, health.calls.Load())
		})
	}
}

func TestTriggerPartialFailure(t *testing.T) {
	inventory, health := &fakePayload{}, &fakePayload{err: errors.New("boom")}
	tp := newTestTriggerPayloads(t, inventory, health)

	err := tp.Trigger(context.Background(), nil)

	var partialErr *rcclienttypes.PartialFailureError
	require.ErrorAs(t, err, &partialErr)
	assert.ErrorContains(t, err, "agent-health: boom")
	assert.NotContains(t, err.Error(), "inventory-agent")
	assert.Equal(t, int32(1), inventory.calls.Load())
	assert.Equal(t, int32(1), health.calls.Load())
}

func TestTriggerUnknownPayload(t *testing.T) {
	inventory, health := &fakePayload{}, &fakePayload{}
	tp := newTestTriggerPayloads(t, inventory, health)

	err := tp.Trigger(context.Background(), []string{"agent-health", "unknown"})

	var partialErr *rcclienttypes.PartialFailureError
	require.ErrorAs(t, err, &partialErr)
	assert.ErrorContains(t, err, "unknown: unknown payload")
	assert.Equal(t, int32(1), health.calls.Load())
}

func TestTriggerAllFailed(t *testing.T) {
	inventory, health := &fakePayload{err: errors.New("boom1")}, &fakePayload{err: errors.New("boom2")}
	tp := newTestTriggerPayloads(t, inventory, health)

	err := tp.Trigger(context.Background(), nil)

	var partialErr *rcclienttypes.PartialFailureError
	require.Error(t, err)
	assert.False(t, errors.As(err, &partialErr))
	assert.ErrorContains(t, err, "inventory-agent: boom1")
	assert.ErrorContains(t, err, "agent-health: boom2")
}

func TestTriggerDoesNotModifyInput(t *testing.T) {
	tp := newTestTriggerPayloads(t, &fakePayload{}, &fakePayload{})
	payloads := []string{"inventory-agent", "agent-health"}

	require.NoError(t, tp.Trigger(context.Background(), payloads))
	assert.Equal(t, []string{"inventory-agent", "agent-health"}, payloads)
}

func TestTriggerParallel(t *testing.T) {
	// Each payload waits for the other one to start, so this only completes if they run concurrently
	var barrier sync.WaitGroup
	barrier.Add(2)
	send := func(context.Context) error {
		barrier.Done()
		barrier.Wait()
		return nil
	}
	tp := &triggerPayloads{
		log: logmock.New(t),
		payloads: map[string]sendFunc{
			triggerpayloads.PayloadInventoryAgent: send,
			triggerpayloads.PayloadAgentHealth:    send,
		},
	}

	done := make(chan error)
	go func() { done <- tp.Trigger(context.Background(), nil) }()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("payloads were not triggered in parallel")
	}
}

func TestTriggerTimeout(t *testing.T) {
	// The fake clock only advances once every goroutine of the bubble is blocked, so the health send always
	// completes before the deadline while the inventory send is still running
	synctest.Test(t, func(t *testing.T) {
		block := make(chan struct{})
		health := &fakePayload{}
		tp := &triggerPayloads{
			log: logmock.New(t),
			payloads: map[string]sendFunc{
				// Ignores the context, like the inventory sender
				triggerpayloads.PayloadInventoryAgent: func(context.Context) error {
					<-block
					return nil
				},
				triggerpayloads.PayloadAgentHealth: health.send,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), triggerTimeout)
		defer cancel()

		err := tp.Trigger(ctx, nil)

		var partialErr *rcclienttypes.PartialFailureError
		require.ErrorAs(t, err, &partialErr)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.ErrorContains(t, err, "inventory-agent")
		assert.NotContains(t, err.Error(), "agent-health")
		assert.Equal(t, int32(1), health.calls.Load())

		// Let the still running sender exit before the bubble ends
		close(block)
		synctest.Wait()
	})
}
