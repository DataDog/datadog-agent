// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package runners

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/queries"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const queriesBundle = "com.datadoghq.remoteaction.queries"

type queryIPC struct {
	ipc.HTTPClient
	post func(context.Context, []byte) ([]byte, error)
}

func (c *queryIPC) Post(address, _ string, body io.Reader, options ...ipc.RequestOption) ([]byte, error) {
	request, err := http.NewRequest(http.MethodPost, address, nil)
	if err != nil {
		return nil, err
	}
	params := ipc.RequestParams{Request: request}
	for _, option := range options {
		option(&params)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return c.post(params.Context(), data)
}
func queryExecutor(t *testing.T, client ipc.HTTPClient, defaultTimeout int32) *WorkflowTaskExecutor {
	t.Helper()
	coreconfig.NewMock(t)
	return &WorkflowTaskExecutor{
		registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{queriesBundle: queries.New(client)}},
		config:   &config.Config{ActionsAllowlist: map[string]sets.Set[string]{queriesBundle: sets.New("execute")}, MetricsClient: &statsd.NoOpClient{}, TaskTimeoutSeconds: &defaultTimeout},
	}
}
func queryTask() *types.Task {
	task := newWorkflowTask("action-task", queriesBundle, "execute", "job-id")
	task.Data.Attributes.Inputs = map[string]interface{}{"integration": "postgres", "target": map[string]interface{}{"database_instance": "orders"}, "resolveOnly": false}
	task.Data.Attributes.TraceId = 1001
	task.Data.Attributes.SpanId = 2002
	return task
}

// RUN-05: the actual queries action controls the shared executor timeout seam.
func TestRUN05(t *testing.T) {
	for _, tc := range []struct {
		name          string
		timeout       interface{}
		expected      time.Duration
		wait, invalid bool
	}{
		{"omitted uses runner default", nil, 60 * time.Second, false, false},
		{"longer task overrides default", float64(3000), 3000 * time.Second, false, false},
		{"shorter task overrides default", float64(2), 2 * time.Second, false, false},
		{"timeout ends action", float64(1), time.Second, true, false},
		{"fraction rejected", 1.5, 0, false, true}, {"string rejected", "60", 0, false, true}, {"zero rejected", float64(0), 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			client := &queryIPC{post: func(ctx context.Context, _ []byte) ([]byte, error) {
				called = true
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.InDelta(t, tc.expected.Seconds(), time.Until(deadline).Seconds(), 0.2)
				if tc.wait {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return []byte(`{"status":"SUCCEEDED"}`), nil
			}}
			executor := queryExecutor(t, client, 60)
			task := queryTask()
			if tc.timeout != nil {
				task.Data.Attributes.Inputs["timeout"] = tc.timeout
			}
			_, err := executor.RunPrepared(context.Background(), &PreparedWorkflowTask{Task: task})
			if tc.invalid {
				require.ErrorContains(t, err, "timeout")
				require.False(t, called)
			} else if tc.wait {
				require.ErrorContains(t, err, "exceeded timeout")
				require.True(t, called)
			} else {
				require.NoError(t, err)
				require.True(t, called)
			}
		})
	}
}

type traceRoundTripper func(*http.Request) (*http.Response, error)

func (f traceRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type recordedSpan struct {
	TraceID  uint64             `json:"trace_id"`
	SpanID   uint64             `json:"span_id"`
	Name     string             `json:"name"`
	Resource string             `json:"resource"`
	Metrics  map[string]float64 `json:"metrics"`
}

// OBS-01: record the runner's emitted spans and compare the propagated active
// span (not the incoming AP parent) and priority; invalid contexts are omitted.
func TestOBS01(t *testing.T) {
	for _, tc := range []struct {
		name     string
		priority *int
		forward  bool
	}{
		{"auto keep", nil, true}, {"priority one", intPtr(1), true}, {"priority two", intPtr(2), true},
		{"drop", intPtr(0), false}, {"unknown priority", intPtr(3), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wire remotequeries.WireRequest
			client := &queryIPC{post: func(_ context.Context, data []byte) ([]byte, error) {
				require.NoError(t, json.Unmarshal(data, &wire))
				return []byte(`{"status":"SUCCEEDED"}`), nil
			}}
			executor := queryExecutor(t, client, 60)
			var traces [][]recordedSpan
			var payload []byte
			recorder := &http.Client{Transport: traceRoundTripper(func(r *http.Request) (*http.Response, error) {
				var err error
				payload, err = io.ReadAll(r.Body)
				require.NoError(t, err)
				var event struct {
					Payload struct {
						Traces [][]recordedSpan `json:"traces"`
					} `json:"payload"`
				}
				require.NoError(t, json.Unmarshal(payload, &event))
				traces = append(traces, event.Payload.Traces...)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			})}
			tel := telemetry.NewTelemetry(recorder, "test-key", "datadoghq.com", observability.ParService)
			ctx := context.Background()
			if tc.priority != nil {
				ctx = telemetry.WithSamplingPriority(ctx, *tc.priority)
			}
			_, err := executor.RunPrepared(ctx, &PreparedWorkflowTask{Task: queryTask()})
			require.NoError(t, err)
			tel.Stop()
			if !tc.forward {
				require.Nil(t, wire.TraceContext)
				return
			}
			require.NotNil(t, wire.TraceContext)
			require.Equal(t, "1001", wire.TraceContext.TraceID)
			require.NotEqual(t, "2002", wire.TraceContext.ParentID)
			found := false
			for _, trace := range traces {
				for _, span := range trace {
					if span.Name == observability.ActionRunOperation && strconv.FormatUint(span.SpanID, 10) == wire.TraceContext.ParentID {
						found = true
						require.EqualValues(t, 1001, span.TraceID)
						require.Equal(t, queriesBundle+".execute", span.Resource)
						require.EqualValues(t, wire.TraceContext.SamplingPriority, span.Metrics["_sampling_priority_v1"])
					}
				}
			}
			require.True(t, found, "active action span was not recorded")
			require.NotContains(t, string(payload), "secret_value")
		})
	}
	// An action invoked with no active runner span also omits traceContext.
	var wire remotequeries.WireRequest
	client := &queryIPC{post: func(_ context.Context, data []byte) ([]byte, error) {
		require.NoError(t, json.Unmarshal(data, &wire))
		return []byte(`{"status":"SUCCEEDED"}`), nil
	}}
	coreconfig.NewMock(t)
	_, err := queries.New(client).GetAction("execute").Run(context.Background(), queryTask(), nil)
	require.NoError(t, err)
	require.Nil(t, wire.TraceContext)
}
func intPtr(v int) *int { return &v }
