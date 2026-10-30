// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	testopms "github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms/testing"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

type investigationAction struct {
	err error
}

func (a investigationAction) Run(ctx context.Context, _ *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	if _, ok := telemetry.SpanFromContext(ctx); !ok {
		return nil, errors.New("diagnostic: action context has no registered span")
	}
	return "synthetic-output", a.err
}

type investigationTransport struct{ collector *url.URL }

func (tr investigationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme, copy.URL.Host = tr.collector.Scheme, tr.collector.Host
	return http.DefaultTransport.RoundTrip(copy)
}

// Diagnostic only: execute fake actions through the shared production seam,
// then explicitly add a sender to distinguish finished spans from delivery.
func TestInvestigationActionSpans(t *testing.T) {
	type wireSpan struct {
		Name     string            `json:"name"`
		Service  string            `json:"service"`
		Resource string            `json:"resource"`
		TraceID  uint64            `json:"trace_id"`
		SpanID   uint64            `json:"span_id"`
		ParentID uint64            `json:"parent_id"`
		Duration int64             `json:"duration"`
		Error    int               `json:"error"`
		Meta     map[string]string `json:"meta"`
	}
	var mu sync.Mutex
	var spans []wireSpan
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event struct {
			Payload struct {
				Traces [][]wireSpan `json:"traces"`
			} `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, trace := range event.Payload.Traces {
			spans = append(spans, trace...)
		}
	}))
	defer collector.Close()
	const bundleID = "com.datadoghq.diagnostic"
	makeExecutor := func(action types.Action) *WorkflowTaskExecutor {
		return &WorkflowTaskExecutor{
			registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{bundleID: &fakeBundle{action: action}}},
			config:   &config.Config{ActionsAllowlist: map[string]sets.Set[string]{bundleID: sets.New("noop")}, MetricsClient: &statsd.NoOpClient{}},
		}
	}
	makePrepared := func(id string) *PreparedWorkflowTask {
		task := newWorkflowTask(id, bundleID, "noop", "synthetic-job")
		task.Data.Attributes.TraceId = 9007199254740993
		task.Data.Attributes.SpanId = 18446744073709551614
		return &PreparedWorkflowTask{Task: task}
	}
	_, err := makeExecutor(investigationAction{}).RunPrepared(context.Background(), makePrepared("success"))
	require.NoError(t, err)
	_, err = makeExecutor(investigationAction{err: errors.New("synthetic failure")}).RunPrepared(context.Background(), makePrepared("error"))
	require.Error(t, err)
	for _, rejection := range []string{"missing-bundle", "missing-action", "allowlist"} {
		executor := makeExecutor(investigationAction{})
		switch rejection {
		case "missing-bundle":
			delete(executor.registry.Bundles, bundleID)
		case "missing-action":
			executor.registry.Bundles[bundleID] = &fakeBundle{}
		case "allowlist":
			delete(executor.config.ActionsAllowlist, bundleID)
		}
		_, err := executor.RunPrepared(context.Background(), makePrepared(rejection))
		require.Error(t, err)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"concurrent-1", "concurrent-2", "concurrent-3"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := makeExecutor(investigationAction{}).RunPrepared(context.Background(), makePrepared(id))
			assert.NoError(t, err)
		}(id)
	}
	wg.Wait()
	mu.Lock()
	require.Empty(t, spans, "span creation/finish alone must not send")
	mu.Unlock()
	collectorURL, err := url.Parse(collector.URL)
	require.NoError(t, err)
	sender := telemetry.NewTelemetry(&http.Client{Transport: investigationTransport{collector: collectorURL}}, "fake-api-key", "datadoghq.com", "private-action-runner")
	sender.Stop()
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, spans, 5, "only executed actions should have finished spans")
	ids := make(map[uint64]bool)
	for _, span := range spans {
		assert.Equal(t, "action.run", span.Name)
		assert.Equal(t, "private-action-runner", span.Service)
		assert.Equal(t, bundleID+".noop", span.Resource)
		assert.Equal(t, uint64(9007199254740993), span.TraceID)
		assert.Equal(t, uint64(18446744073709551614), span.ParentID)
		assert.NotZero(t, span.SpanID)
		assert.False(t, ids[span.SpanID])
		ids[span.SpanID] = true
		assert.Positive(t, span.Duration)
		if span.Meta["task_id"] == "error" {
			assert.Equal(t, 1, span.Error)
		} else {
			assert.Zero(t, span.Error)
		}
	}
	t.Log("five completed action.run spans reached loopback only after explicitly starting/stopping a sender; pre-execution rejections produced none")
}

type fakeTaskVerifier struct {
	unwrappedTask *types.Task
	err           error
	gotTask       *types.Task
}

func (f *fakeTaskVerifier) UnwrapTask(task *types.Task) (*types.Task, error) {
	f.gotTask = task
	return f.unwrappedTask, f.err
}

type fakeCredentialResolver struct {
	credential *privateconnection.PrivateCredentials
	err        error
	gotConn    *privateactionspb.ConnectionInfo
}

type fakeBundle struct {
	action types.Action
}

func (b *fakeBundle) GetAction(string) types.Action {
	return b.action
}

type recordingAction struct {
	called bool
}

func (a *recordingAction) Run(context.Context, *types.Task, *privateconnection.PrivateCredentials) (interface{}, error) {
	a.called = true
	return "executed", nil
}

func (f *fakeCredentialResolver) ResolveConnectionInfoToCredential(_ context.Context, conn *privateactionspb.ConnectionInfo, _ *uuid.UUID) (*privateconnection.PrivateCredentials, error) {
	f.gotConn = conn
	return f.credential, f.err
}

func TestWorkflowTaskExecutorPrepareTaskPreservesDequeueMetadataAndResolvesCredentials(t *testing.T) {
	dequeuedTask := newWorkflowTask("wrapper-task", "wrapper-bundle", "wrapper-action", "job-id")
	dequeuedTask.Data.Attributes.TraceId = 123
	dequeuedTask.Data.Attributes.SpanId = 456
	envelope := &privateactionspb.RemoteConfigSignatureEnvelope{Data: []byte("signed")}
	dequeuedTask.Data.Attributes.SignedEnvelope = envelope

	connectionInfo := &privateactionspb.ConnectionInfo{RunnerId: "runner-id"}
	unwrappedTask := newWorkflowTask("signed-task", "signed-bundle", "signed-action", "")
	unwrappedTask.Data.Attributes.ConnectionInfo = connectionInfo
	credential := &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "token", Value: "value"},
		},
	}
	verifier := &fakeTaskVerifier{unwrappedTask: unwrappedTask}
	credentialResolver := &fakeCredentialResolver{credential: credential}
	executor := &WorkflowTaskExecutor{
		taskVerifier: verifier,
		resolver:     credentialResolver,
	}

	got, failureTask, err := executor.PrepareTask(context.Background(), dequeuedTask)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, failureTask)
	assert.Same(t, dequeuedTask, verifier.gotTask)
	assert.Same(t, connectionInfo, credentialResolver.gotConn)
	assert.Same(t, unwrappedTask, got.Task)
	assert.Same(t, credential, got.Credential)
	assert.Equal(t, "job-id", got.Task.Data.Attributes.JobId)
	assert.Equal(t, uint64(123), got.Task.Data.Attributes.TraceId)
	assert.Equal(t, uint64(456), got.Task.Data.Attributes.SpanId)
	assert.Same(t, envelope, got.Task.Data.Attributes.SignedEnvelope)
}

func TestWorkflowTaskExecutorPrepareTaskReturnsOriginalTaskWhenVerificationFails(t *testing.T) {
	verifyErr := errors.New("verification failed")
	dequeuedTask := newWorkflowTask("wrapper-task", "wrapper-bundle", "wrapper-action", "job-id")
	verifier := &fakeTaskVerifier{err: verifyErr}
	credentialResolver := &fakeCredentialResolver{}
	executor := &WorkflowTaskExecutor{
		taskVerifier: verifier,
		resolver:     credentialResolver,
	}

	got, failureTask, err := executor.PrepareTask(context.Background(), dequeuedTask)

	require.ErrorIs(t, err, verifyErr)
	assert.Nil(t, got)
	assert.Same(t, dequeuedTask, failureTask)
	assert.Nil(t, credentialResolver.gotConn)
}

func TestWorkflowTaskExecutorPrepareTaskReturnsUnwrappedTaskWhenCredentialResolutionFails(t *testing.T) {
	resolveErr := errors.New("credential resolution failed")
	dequeuedTask := newWorkflowTask("wrapper-task", "wrapper-bundle", "wrapper-action", "job-id")
	dequeuedTask.Data.Attributes.TraceId = 123
	dequeuedTask.Data.Attributes.SpanId = 456
	unwrappedTask := newWorkflowTask("signed-task", "signed-bundle", "signed-action", "")
	verifier := &fakeTaskVerifier{unwrappedTask: unwrappedTask}
	executor := &WorkflowTaskExecutor{
		taskVerifier: verifier,
		resolver:     &fakeCredentialResolver{err: resolveErr},
	}

	got, failureTask, err := executor.PrepareTask(context.Background(), dequeuedTask)

	require.ErrorIs(t, err, resolveErr)
	assert.Nil(t, got)
	assert.Same(t, unwrappedTask, failureTask)
	assert.Equal(t, "job-id", failureTask.Data.Attributes.JobId)
	assert.Equal(t, uint64(123), failureTask.Data.Attributes.TraceId)
	assert.Equal(t, uint64(456), failureTask.Data.Attributes.SpanId)
}

func TestWorkflowTaskExecutorEnforcesAuthoredScriptAllowlist(t *testing.T) {
	const (
		bundleID   = "com.datadoghq.authoredscripts"
		actionName = "echo"
	)
	action := &recordingAction{}
	executor := &WorkflowTaskExecutor{
		registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{
			bundleID: &fakeBundle{action: action},
		}},
		config: &config.Config{
			ActionsAllowlist: map[string]sets.Set[string]{},
			MetricsClient:    &statsd.NoOpClient{},
		},
	}
	task := newWorkflowTask("task-id", bundleID, actionName, "job-id")
	prepared := &PreparedWorkflowTask{Task: task}

	_, err := executor.RunTask(context.Background(), prepared)
	require.ErrorContains(t, err, "not allowlisted")
	assert.False(t, action.called)

	executor.config.ActionsAllowlist[bundleID] = sets.New(actionName)
	output, err := executor.RunTask(context.Background(), prepared)
	require.NoError(t, err)
	assert.Equal(t, "executed", output)
	assert.True(t, action.called)
}

func TestWorkflowRunnerPublishesFailureWhenTaskPreparationFails(t *testing.T) {
	verifyErr := errors.New("verification failed")
	dequeuedTask := newWorkflowTask("wrapper-task", "wrapper-bundle", "wrapper-action", "job-id")
	published := make(chan struct{}, 1)
	shutdown := make(chan struct{})
	opmsClient := &testopms.FakeOpmsClient{
		DequeueTaskFn: func(_ context.Context) (*types.Task, time.Duration, error) {
			return dequeuedTask, 0, nil
		},
		PublishFailureFn: func(_ context.Context, _ actionsclientpb.Client, taskID, jobID, actionFQN string, errorCode aperrorpb.ActionPlatformErrorCode, _, _ string) error {
			assert.Equal(t, "wrapper-task", taskID)
			assert.Equal(t, "job-id", jobID)
			assert.Equal(t, "wrapper-bundle.wrapper-action", actionFQN)
			assert.Equal(t, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errorCode)
			close(shutdown)
			published <- struct{}{}
			return nil
		},
	}
	runner := &WorkflowRunner{
		config: &config.Config{
			LoopInterval: time.Hour,
			MaxAttempts:  1,
		},
		opmsClient:      opmsClient,
		taskExecutor:    &WorkflowTaskExecutor{taskVerifier: &fakeTaskVerifier{err: verifyErr}},
		sem:             make(chan struct{}, 1),
		shutdownChannel: shutdown,
	}
	done := make(chan struct{})
	go func() {
		runner.run(context.Background())
		close(done)
	}()

	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for publish failure")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runner loop to stop")
	}
}

func TestWorkflowRunner_StartHeartbeat_StopsOnJobNotFound(t *testing.T) {
	runner := &WorkflowRunner{
		config: &config.Config{HeartbeatInterval: 10 * time.Millisecond},
		opmsClient: &testopms.FakeOpmsClient{
			HeartbeatFn: func(_ context.Context, _ actionsclientpb.Client, _, _, _ string) error {
				return opms.ErrJobNotFound
			},
		},
	}
	task := newWorkflowTask("task-id", "com.datadoghq.test", "action", "job-id")

	done := make(chan struct{})
	go func() {
		runner.startHeartbeat(context.Background(), task, log.FromContext(context.Background()))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat did not stop after job not found")
	}
}

func newWorkflowTask(taskID, bundleID, actionName, jobID string) *types.Task {
	task := &types.Task{}
	task.Data.ID = taskID
	task.Data.Attributes = &types.Attributes{
		BundleID: bundleID,
		Name:     actionName,
		JobId:    jobID,
	}
	return task
}
