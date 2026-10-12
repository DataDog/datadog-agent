// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
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
	gotAction  resolver.ActionContext
	called     bool
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

func (f *fakeCredentialResolver) ResolveConnectionInfoToCredential(_ context.Context, conn *privateactionspb.ConnectionInfo, action resolver.ActionContext, _ *uuid.UUID) (*privateconnection.PrivateCredentials, error) {
	f.gotConn = conn
	f.gotAction = action
	f.called = true
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
	dequeuedTask.Data.Attributes.Inputs = map[string]interface{}{"url": "https://unsigned.example.com"}
	unwrappedTask.Data.Attributes.Inputs = map[string]interface{}{"url": "https://signed.example.com"}
	credential := &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "token", Value: "value"},
		},
	}
	verifier := &fakeTaskVerifier{unwrappedTask: unwrappedTask}
	credentialResolver := &fakeCredentialResolver{credential: credential}
	executor := &WorkflowTaskExecutor{
		registry:     &privatebundles.Registry{Bundles: map[string]types.Bundle{"signed-bundle": &fakeBundle{action: &recordingAction{}}}},
		config:       &config.Config{ActionsAllowlist: map[string]sets.Set[string]{"signed-bundle": sets.New("signed-action")}},
		taskVerifier: verifier,
		resolver:     credentialResolver,
	}

	got, failureTask, err := executor.PrepareTask(context.Background(), dequeuedTask)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, failureTask)
	assert.Same(t, dequeuedTask, verifier.gotTask)
	assert.Same(t, connectionInfo, credentialResolver.gotConn)
	assert.Equal(t, resolver.ActionContext{BundleID: "signed-bundle", ActionName: "signed-action", HTTPRequestURL: "https://signed.example.com"}, credentialResolver.gotAction)
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
		registry:     &privatebundles.Registry{Bundles: map[string]types.Bundle{"signed-bundle": &fakeBundle{action: &recordingAction{}}}},
		config:       &config.Config{ActionsAllowlist: map[string]sets.Set[string]{"signed-bundle": sets.New("signed-action")}},
		taskVerifier: verifier,
		resolver:     credentialResolver,
	}

	got, failureTask, err := executor.PrepareTask(context.Background(), dequeuedTask)

	require.ErrorIs(t, err, verifyErr)
	assert.Nil(t, got)
	assert.Same(t, dequeuedTask, failureTask)
	assert.False(t, credentialResolver.called)
}

func TestWorkflowTaskExecutorPrepareTaskReturnsUnwrappedTaskWhenCredentialResolutionFails(t *testing.T) {
	resolveErr := errors.New("credential resolution failed")
	dequeuedTask := newWorkflowTask("wrapper-task", "wrapper-bundle", "wrapper-action", "job-id")
	dequeuedTask.Data.Attributes.TraceId = 123
	dequeuedTask.Data.Attributes.SpanId = 456
	unwrappedTask := newWorkflowTask("signed-task", "signed-bundle", "signed-action", "")
	verifier := &fakeTaskVerifier{unwrappedTask: unwrappedTask}
	executor := &WorkflowTaskExecutor{
		registry:     &privatebundles.Registry{Bundles: map[string]types.Bundle{"signed-bundle": &fakeBundle{action: &recordingAction{}}}},
		config:       &config.Config{ActionsAllowlist: map[string]sets.Set[string]{"signed-bundle": sets.New("signed-action")}},
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

func TestPrepareTaskRejectsActionBeforeResolvingCredentials(t *testing.T) {
	for _, test := range []struct {
		name      string
		bundle    types.Bundle
		allowed   bool
		wantError string
	}{
		{name: "unknown bundle", allowed: true, wantError: "could not find bundle"},
		{name: "unknown action", bundle: &fakeBundle{}, allowed: true, wantError: "could not find action"},
		{name: "disabled action", bundle: &fakeBundle{action: &recordingAction{}}, wantError: "not allowlisted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			credentialResolver := &fakeCredentialResolver{}
			unwrapped := newWorkflowTask("signed-task", "com.datadoghq.mongodb", "find", "")
			executor := &WorkflowTaskExecutor{
				registry:     &privatebundles.Registry{Bundles: map[string]types.Bundle{"com.datadoghq.mongodb": test.bundle}},
				config:       &config.Config{ActionsAllowlist: map[string]sets.Set[string]{"com.datadoghq.http": sets.New("sendRequest")}},
				taskVerifier: &fakeTaskVerifier{unwrappedTask: unwrapped},
				resolver:     credentialResolver,
			}
			if test.allowed {
				executor.config.ActionsAllowlist["com.datadoghq.mongodb"] = sets.New("find")
			}
			prepared, failureTask, err := executor.PrepareTask(context.Background(), newWorkflowTask("wrapper", "com.datadoghq.http", "sendRequest", "job-id"))
			require.ErrorContains(t, err, test.wantError)
			assert.Nil(t, prepared)
			assert.Same(t, unwrapped, failureTask)
			assert.Equal(t, "job-id", failureTask.Data.Attributes.JobId)
			assert.False(t, credentialResolver.called)
		})
	}
}

func TestPrepareTaskEnforcesCredentialPolicyUsingVerifiedIdentity(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(strconv.FormatBool(allowed), func(t *testing.T) {
			credentialResolver, err := resolver.NewPrivateCredentialResolver(configmock.NewFromYAML(t, `
private_action_runner:
  credentials:
    values:
      password:
        value: secret-value
        allowed_actions: [com.datadoghq.mongodb.*]
`))
			require.NoError(t, err)
			mongodb := newWorkflowTask("mongo", "com.datadoghq.mongodb", "find", "job-id")
			http := newWorkflowTask("http", "com.datadoghq.http", "sendRequest", "job-id")
			wrapper, verified := mongodb, http
			if allowed {
				wrapper, verified = http, mongodb
			}
			verified.Data.Attributes.ConnectionInfo = &privateactionspb.ConnectionInfo{
				CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
				TokensV2: []*privateactionspb.ConnectionTokenV2{{
					NameSegments: []string{"root_tokens", "password"},
					Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
						RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "password"},
					},
				}},
			}
			executor := &WorkflowTaskExecutor{
				registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{
					"com.datadoghq.mongodb": &fakeBundle{action: &recordingAction{}},
					"com.datadoghq.http":    &fakeBundle{action: &recordingAction{}},
				}},
				config: &config.Config{ActionsAllowlist: map[string]sets.Set[string]{
					"com.datadoghq.mongodb": sets.New("*"),
					"com.datadoghq.http":    sets.New("*"),
				}},
				taskVerifier: &fakeTaskVerifier{unwrappedTask: verified},
				resolver:     credentialResolver,
			}
			prepared, failureTask, err := executor.PrepareTask(context.Background(), wrapper)
			if allowed {
				require.NoError(t, err)
				assert.Nil(t, failureTask)
				require.NotNil(t, prepared)
				require.Len(t, prepared.Credential.Tokens, 1)
				assert.Equal(t, "secret-value", prepared.Credential.Tokens[0].Value)
			} else {
				require.ErrorContains(t, err, "not allowed for action")
				assert.NotContains(t, err.Error(), "secret-value")
				assert.Nil(t, prepared)
				assert.Same(t, verified, failureTask)
			}
		})
	}
}
