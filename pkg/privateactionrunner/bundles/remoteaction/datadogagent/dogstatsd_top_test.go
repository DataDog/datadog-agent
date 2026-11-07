// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_datadogagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/contexttop"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestGetDogstatsdTopHandlerRunsAgentCLI(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]interface{}
		wantArgs []string
	}{
		{
			name:   "defaults",
			inputs: map[string]interface{}{},
			wantArgs: []string{
				"dogstatsd", "top", "--path", "/opt/datadog-agent/run/dogstatsd_contexts.json.zstd",
				"--json", "-m", "10", "-t", "5",
			},
		},
		{
			name:   "explicit limits",
			inputs: map[string]interface{}{"num_metrics": 20, "num_tags": 8, "source": "dump"},
			wantArgs: []string{
				"dogstatsd", "top", "--path", "/opt/datadog-agent/run/dogstatsd_contexts.json.zstd",
				"--json", "-m", "20", "-t", "8",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeIPCClient{
				get: func(endpointURL string, _ ...ipc.RequestOption) ([]byte, error) {
					require.True(t, strings.HasSuffix(endpointURL, "/agent/dogstatsd-contexts-dump"))
					return []byte(`{
						"path":"/opt/datadog-agent/run/dogstatsd_contexts.json.zstd",
						"size":1234,
						"modified_at_unix_nano":42
					}`), nil
				},
			}
			handler := NewGetDogstatsdTopHandler(client)
			handler.agentBinary = "/opt/datadog-agent/bin/agent/agent"
			handler.runCommand = func(_ context.Context, binary string, args ...string) (string, error) {
				require.Equal(t, "/opt/datadog-agent/bin/agent/agent", binary)
				require.Equal(t, test.wantArgs, args)
				return `{"metrics":[{"name":"requests","contexts":2,"tags":[]}]}`, nil
			}

			result, err := handler.Run(context.Background(), newDogstatsdTopTask(test.inputs), nil)
			require.NoError(t, err)
			require.Equal(t, dogstatsdTopResponse{
				Source: dogstatsdTopSourceDump,
				Result: contexttop.Result{Metrics: []contexttop.Metric{{
					Name: "requests", Contexts: 2, Tags: []contexttop.Tag{},
				}}},
			}, result)
		})
	}
}

func TestGetDogstatsdTopHandlerValidatesInputsBeforeAgentRequest(t *testing.T) {
	tests := []map[string]interface{}{
		{"source": "live"},
		{"num_metrics": 0},
		{"num_metrics": maxDogstatsdTopNumMetrics + 1},
		{"num_tags": 0},
		{"num_tags": maxDogstatsdTopNumTags + 1},
	}

	for _, inputs := range tests {
		handler := NewGetDogstatsdTopHandler(&fakeIPCClient{})
		_, err := handler.Run(context.Background(), newDogstatsdTopTask(inputs), nil)
		require.Error(t, err)
	}
}

func TestGetDogstatsdTopHandlerReturnsAgentAndChildErrors(t *testing.T) {
	t.Run("agent error", func(t *testing.T) {
		handler := NewGetDogstatsdTopHandler(&fakeIPCClient{
			get: func(string, ...ipc.RequestOption) ([]byte, error) {
				return []byte(`{"error":"DogStatsD contexts dump has not been created"}`), errors.New("not found")
			},
		})
		_, err := handler.Run(context.Background(), newDogstatsdTopTask(nil), nil)
		require.ErrorContains(t, err, "DogStatsD contexts dump has not been created")
	})

	t.Run("child error", func(t *testing.T) {
		handler := newDogstatsdTopTestHandler(t)
		handler.runCommand = func(context.Context, string, ...string) (string, error) {
			return "", errors.New("child failed")
		}
		_, err := handler.Run(context.Background(), newDogstatsdTopTask(nil), nil)
		require.ErrorContains(t, err, "child failed")
	})

	t.Run("invalid child output", func(t *testing.T) {
		handler := newDogstatsdTopTestHandler(t)
		handler.runCommand = func(context.Context, string, ...string) (string, error) {
			return "not json", nil
		}
		_, err := handler.Run(context.Background(), newDogstatsdTopTask(nil), nil)
		require.ErrorContains(t, err, "invalid JSON from agent CLI")
	})
}

func TestRunDogstatsdTopCommand(t *testing.T) {
	output, err := runDogstatsdTopCommand(
		context.Background(),
		os.Args[0],
		"-test.run=^TestDogstatsdTopCommandHelper$", "--", "success",
	)
	require.NoError(t, err)
	require.Contains(t, output, `{"metrics":[]}`)

	_, err = runDogstatsdTopCommand(
		context.Background(),
		os.Args[0],
		"-test.run=^TestDogstatsdTopCommandHelper$", "--", "failure",
	)
	require.ErrorContains(t, err, "agent CLI failed")
	require.ErrorContains(t, err, "boom")
}

func TestDogstatsdTopCommandHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "success":
		fmt.Fprint(os.Stdout, `{"metrics":[]}`)
	case "failure":
		fmt.Fprint(os.Stderr, "boom")
		os.Exit(3)
	}
}

func TestDogstatsdTopCoordinatorCoalescesIdenticalRequests(t *testing.T) {
	coordinator := newDogstatsdTopCoordinator()
	key := dogstatsdTopKey{path: "dump", size: 10, modifiedAtUnixNano: 20, numMetrics: 10, numTags: 5}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	operation := func(context.Context) (dogstatsdTopResponse, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return dogstatsdTopResponse{Source: dogstatsdTopSourceDump}, nil
	}

	results := make(chan dogstatsdTopResponse, 2)
	errs := make(chan error, 2)
	go func() {
		result, err := coordinator.Do(context.Background(), key, operation)
		results <- result
		errs <- err
	}()
	<-started
	go func() {
		result, err := coordinator.Do(context.Background(), key, operation)
		results <- result
		errs <- err
	}()
	require.Eventually(t, func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		return coordinator.flights[key] != nil && coordinator.flights[key].waiters == 2
	}, time.Second, time.Millisecond)
	close(release)

	for range 2 {
		require.NoError(t, <-errs)
		require.Equal(t, dogstatsdTopSourceDump, (<-results).Source)
	}
	require.Equal(t, int32(1), calls.Load())
}

func TestDogstatsdTopCoordinatorSerializesDifferentRequests(t *testing.T) {
	coordinator := newDogstatsdTopCoordinator()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	done := make(chan error, 2)

	go func() {
		_, err := coordinator.Do(context.Background(), dogstatsdTopKey{path: "first"}, func(context.Context) (dogstatsdTopResponse, error) {
			close(firstStarted)
			<-releaseFirst
			return dogstatsdTopResponse{}, nil
		})
		done <- err
	}()
	<-firstStarted
	go func() {
		_, err := coordinator.Do(context.Background(), dogstatsdTopKey{path: "second"}, func(context.Context) (dogstatsdTopResponse, error) {
			close(secondStarted)
			<-releaseSecond
			return dogstatsdTopResponse{}, nil
		})
		done <- err
	}()

	require.Eventually(t, func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		return coordinator.flights[dogstatsdTopKey{path: "second"}] != nil
	}, time.Second, time.Millisecond)
	select {
	case <-secondStarted:
		t.Fatal("second operation started while the first child slot was occupied")
	default:
	}
	close(releaseFirst)
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second operation did not start after the child slot was released")
	}
	close(releaseSecond)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
}

func TestDogstatsdTopCoordinatorKeepsSharedOperationForRemainingWaiter(t *testing.T) {
	coordinator := newDogstatsdTopCoordinator()
	key := dogstatsdTopKey{path: "dump"}
	started := make(chan struct{})
	release := make(chan struct{})
	operationCanceled := make(chan struct{})
	operation := func(ctx context.Context) (dogstatsdTopResponse, error) {
		close(started)
		select {
		case <-release:
			return dogstatsdTopResponse{Source: dogstatsdTopSourceDump}, nil
		case <-ctx.Done():
			close(operationCanceled)
			return dogstatsdTopResponse{}, ctx.Err()
		}
	}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := coordinator.Do(firstCtx, key, operation)
		firstDone <- err
	}()
	<-started
	go func() {
		_, err := coordinator.Do(context.Background(), key, operation)
		secondDone <- err
	}()
	require.Eventually(t, func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		return coordinator.flights[key] != nil && coordinator.flights[key].waiters == 2
	}, time.Second, time.Millisecond)

	cancelFirst()
	require.ErrorIs(t, <-firstDone, context.Canceled)
	select {
	case <-operationCanceled:
		t.Fatal("shared operation was canceled while another caller was waiting")
	default:
	}
	close(release)
	require.NoError(t, <-secondDone)
}

func newDogstatsdTopTask(inputs map[string]interface{}) *types.Task {
	if inputs == nil {
		inputs = map[string]interface{}{}
	}
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: inputs}
	return task
}

func newDogstatsdTopTestHandler(t *testing.T) *GetDogstatsdTopHandler {
	t.Helper()
	return NewGetDogstatsdTopHandler(&fakeIPCClient{
		get: func(string, ...ipc.RequestOption) ([]byte, error) {
			return []byte(`{
				"path":"/opt/datadog-agent/run/dogstatsd_contexts.json.zstd",
				"size":1234,
				"modified_at_unix_nano":42
			}`), nil
		},
	})
}
