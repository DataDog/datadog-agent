// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	testopms "github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms/testing"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestStopFlushesTracesAfterDraining(t *testing.T) {
	for _, mode := range []string{"executor", "monolithic"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				testStopFlushesTracesAfterDraining(t, mode == "executor")
			})
		})
	}
}

func testStopFlushesTracesAfterDraining(t *testing.T, executorMode bool) {
	t.Helper()
	type span struct {
		Service  string `json:"service"`
		Name     string `json:"name"`
		TraceID  uint64 `json:"trace_id"`
		SpanID   uint64 `json:"span_id"`
		ParentID uint64 `json:"parent_id"`
		Duration int64  `json:"duration"`
		Error    int    `json:"error"`
	}
	payloads := make(chan [][]span, 10)
	client := &http.Client{Transport: telemetryTestTransport(func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		assert.Equal(t, "/api/v2/apmtelemetry", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("dd-api-key"))
		var body struct {
			RequestType string `json:"request_type"`
			Payload     struct {
				Traces [][]span `json:"traces"`
			} `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); !assert.NoError(t, err) {
			return nil, err
		}
		assert.Equal(t, "traces", body.RequestType)
		payloads <- body.Payload.Traces
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}
	runner := newStartedRunnerForStopTest(nil, false)
	runner.logger = logmock.New(t)
	runner.telemetry = telemetry.NewTelemetry(client, "test-api-key", "datadoghq.com", observability.ParService)

	// Completed child spans must stay buffered until outstanding synthetic parents finish.
	ready := make(chan struct{}, 2)
	finish := make(chan struct{})
	var tasks sync.WaitGroup
	for i := range 2 {
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			ctx := telemetry.WithService(context.Background(), observability.ParService)
			action, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, uint64(9007199254740993+i), 42)
			child, _ := telemetry.StartSpanFromContext(telemetry.WithService(ctx, observability.RshellService), "rshell.run")
			var actionErr error
			if i == 1 {
				actionErr = errors.New("test action error")
			}
			time.Sleep(time.Millisecond) // Virtual time gives the spans positive durations.
			child.Finish(actionErr)
			ready <- struct{}{}
			<-finish
			action.Finish(actionErr)
		}()
	}
	<-ready
	<-ready
	stop := runner.StopExecutor
	if executorMode {
		runner.executorDone = make(chan struct{})
		go func() {
			tasks.Wait()
			close(runner.executorDone)
		}()
	} else {
		// Use the real workflow wait group: Stop must wait for an in-flight poll.
		// CommonRunner.Stop only cancels its loop; it does not wait for it.
		cfg := coreconfig.NewMock(t)
		runnerConfig, err := parconfig.FromDDConfig(cfg, nil)
		require.NoError(t, err)
		pollStarted := make(chan struct{})
		markPollStarted := sync.OnceFunc(func() { close(pollStarted) })
		opmsClient := &testopms.FakeOpmsClient{
			DequeueTaskFn: func(context.Context) (*types.Task, time.Duration, error) {
				markPollStarted()
				tasks.Wait()
				return nil, 0, nil
			},
		}
		runner.workflowRunner, err = runners.NewWorkflowRunner(runnerConfig, telemetryTestRCClient{}, telemetryTestKeysManager{}, nil, opmsClient, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, runner.workflowRunner.Start(context.Background()))
		<-pollStarted
		stop = runner.Stop
	}
	stopped := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopped <- stop(ctx)
	}()
	synctest.Wait()
	assert.Empty(t, stopped, "shutdown must wait for blocked work")
	assert.NotNil(t, runner.telemetry, "telemetry must remain active while draining")
	assert.Empty(t, payloads, "completed spans must not flush before draining finishes")
	close(finish)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("runner shutdown did not finish")
	}

	require.Nil(t, runner.telemetry)
	// Only telemetry cleanup is repeatable; WorkflowRunner.Stop closes its shutdown channel.
	runner.stopTelemetry()

	// No periodic tick is needed: shutdown must export all finished spans now.
	require.Len(t, payloads, 1)
	traces := <-payloads
	require.Len(t, traces, 2)
	ids := make(map[uint64]bool)
	for _, trace := range traces {
		require.Len(t, trace, 2)
		var action, child span
		for _, s := range trace {
			assert.Positive(t, s.Duration)
			assert.NotZero(t, s.SpanID)
			assert.False(t, ids[s.SpanID])
			ids[s.SpanID] = true
			if s.Name == observability.ActionRunOperation {
				action = s
			} else {
				child = s
			}
		}
		assert.Equal(t, observability.ParService, action.Service)
		assert.Equal(t, observability.RshellService, child.Service)
		assert.Equal(t, action.TraceID, child.TraceID)
		assert.Equal(t, action.SpanID, child.ParentID)
		assert.Equal(t, uint64(42), action.ParentID)
		assert.Contains(t, []uint64{9007199254740993, 9007199254740994}, action.TraceID)
		wantError := int(action.TraceID - 9007199254740993)
		assert.Equal(t, wantError, action.Error)
		assert.Equal(t, wantError, child.Error)
	}
}

type telemetryTestTransport func(*http.Request) (*http.Response, error)

func (f telemetryTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type telemetryTestRCClient struct{}

func (telemetryTestRCClient) Subscribe(string, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}
func (telemetryTestRCClient) GetConfigTUFProof(string) (state.ConfigTUFProof, bool) {
	return state.ConfigTUFProof{}, false
}

type telemetryTestKeysManager struct{}

func (telemetryTestKeysManager) Start(context.Context) {}
func (telemetryTestKeysManager) WaitForReady()         {}
func (telemetryTestKeysManager) GetKey(string) (types.DecodedKey, *types.DirectorKeyProof) {
	return nil, nil
}
