// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

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
	action        types.Action
	gotActionName string
}

func (b *fakeBundle) GetAction(actionName string) types.Action {
	b.gotActionName = actionName
	return b.action
}

type recordingAction struct {
	called bool
	task   *types.Task
}

func (a *recordingAction) Run(_ context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	a.called = true
	a.task = task
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

func TestWorkflowTaskExecutorRoutesByExecutionFQNAndAuthorizesPublicFQN(t *testing.T) {
	const (
		publicBundle   = "com.datadoghq.helm"
		actionName     = "addRepo"
		publicFQN      = publicBundle + "." + actionName
		scriptBundle   = "com.datadoghq.authoredscripts"
		scriptAliasFQN = scriptBundle + ".helm." + actionName
	)

	tests := []struct {
		name                 string
		executionFQN         string
		allowlist            map[string]sets.Set[string]
		wantNative           bool
		wantScript           bool
		wantNativeActionName string
		wantScriptActionName string
		wantError            string
	}{
		{
			name:                 "native action uses public FQN when execution FQN is absent",
			allowlist:            map[string]sets.Set[string]{publicBundle: sets.New(actionName)},
			wantNative:           true,
			wantNativeActionName: actionName,
		},
		{
			name:                 "native action uses public FQN when execution FQN matches it",
			executionFQN:         publicFQN,
			allowlist:            map[string]sets.Set[string]{publicBundle: sets.New(actionName)},
			wantNative:           true,
			wantNativeActionName: actionName,
		},
		{
			name:                 "product action uses authored-script execution FQN",
			executionFQN:         scriptAliasFQN,
			allowlist:            map[string]sets.Set[string]{publicBundle: sets.New(actionName)},
			wantScript:           true,
			wantScriptActionName: actionName,
		},
		{
			name:         "unknown execution FQN fails closed",
			executionFQN: "com.datadoghq.unknown.action",
			allowlist:    map[string]sets.Set[string]{publicBundle: sets.New(actionName)},
			wantError:    "could not find bundle for com.datadoghq.unknown",
		},
		{
			name:                 "execution FQN allowlist entry cannot authorize public action",
			executionFQN:         scriptAliasFQN,
			allowlist:            map[string]sets.Set[string]{scriptBundle + ".helm": sets.New(actionName)},
			wantError:            "not allowlisted",
			wantScriptActionName: actionName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nativeAction := &recordingAction{}
			scriptAction := &recordingAction{}
			nativeBundle := &fakeBundle{action: nativeAction}
			scriptBundleHandler := &fakeBundle{action: scriptAction}
			executor := &WorkflowTaskExecutor{
				registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{
					publicBundle: nativeBundle,
					scriptBundle: scriptBundleHandler,
				}},
				config: &config.Config{
					ActionsAllowlist: tt.allowlist,
					MetricsClient:    &statsd.NoOpClient{},
				},
			}
			task := newWorkflowTask("task-id", publicBundle, actionName, "job-id")
			task.Data.Attributes.ExecutionFQN = tt.executionFQN

			output, err := executor.RunTask(context.Background(), &PreparedWorkflowTask{Task: task})
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				assert.Nil(t, output)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "executed", output)
			}
			assert.Equal(t, tt.wantNative, nativeAction.called)
			assert.Equal(t, tt.wantScript, scriptAction.called)
			assert.Equal(t, tt.wantNativeActionName, nativeBundle.gotActionName)
			assert.Equal(t, tt.wantScriptActionName, scriptBundleHandler.gotActionName)
			if tt.wantScript {
				require.Same(t, task, scriptAction.task)
				assert.Equal(t, publicBundle+"."+actionName, scriptAction.task.GetFQN())
			}
		})
	}
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
