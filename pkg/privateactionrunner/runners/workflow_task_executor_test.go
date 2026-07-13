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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
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

func (f *fakeCredentialResolver) ResolveConnectionInfoToCredential(_ context.Context, conn *privateactionspb.ConnectionInfo, _ *uuid.UUID) (*privateconnection.PrivateCredentials, error) {
	f.gotConn = conn
	return f.credential, f.err
}

func TestWorkflowTaskExecutorPrepareTaskPreservesDequeueMetadataAndResolvesCredentials(t *testing.T) {
	dequeuedTask := newWorkflowTask("wrapper-task", "wrapper-bundle", "wrapper-action", "job-id")
	dequeuedTask.Data.Attributes.TraceId = 123
	dequeuedTask.Data.Attributes.SpanId = 456

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
