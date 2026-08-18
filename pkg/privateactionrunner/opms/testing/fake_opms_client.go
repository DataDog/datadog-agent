// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

// FakeOpmsClient is a configurable opms.Client for use in runner unit tests.
// Set the function fields to override individual methods; unset fields return
// safe zero-value defaults.
type FakeOpmsClient struct {
	HealthCheckFn    func(ctx context.Context) (*opms.HealthCheckData, error)
	DequeueTaskFn    func(ctx context.Context) (*types.Task, time.Duration, error)
	PublishSuccessFn func(ctx context.Context, client actionsclientpb.Client, taskID, jobID, actionFQN string, output interface{}, branch string) error
	PublishFailureFn func(ctx context.Context, client actionsclientpb.Client, taskID, jobID, actionFQN string, errorCode aperrorpb.ActionPlatformErrorCode, errorDetails, apiError string) error
	HeartbeatFn      func(ctx context.Context, client actionsclientpb.Client, taskID, actionFQN, jobID string) error
}

func (f *FakeOpmsClient) HealthCheck(ctx context.Context) (*opms.HealthCheckData, error) {
	if f.HealthCheckFn != nil {
		return f.HealthCheckFn(ctx)
	}
	return &opms.HealthCheckData{}, nil
}

func (f *FakeOpmsClient) DequeueTask(ctx context.Context) (*types.Task, time.Duration, error) {
	if f.DequeueTaskFn != nil {
		return f.DequeueTaskFn(ctx)
	}
	return nil, 0, nil
}

func (f *FakeOpmsClient) PublishSuccess(ctx context.Context, client actionsclientpb.Client, taskID, jobID, actionFQN string, output interface{}, branch string) error {
	if f.PublishSuccessFn != nil {
		return f.PublishSuccessFn(ctx, client, taskID, jobID, actionFQN, output, branch)
	}
	return nil
}

func (f *FakeOpmsClient) PublishFailure(ctx context.Context, client actionsclientpb.Client, taskID, jobID, actionFQN string, errorCode aperrorpb.ActionPlatformErrorCode, errorDetails, apiError string) error {
	if f.PublishFailureFn != nil {
		return f.PublishFailureFn(ctx, client, taskID, jobID, actionFQN, errorCode, errorDetails, apiError)
	}
	return nil
}

func (f *FakeOpmsClient) Heartbeat(ctx context.Context, client actionsclientpb.Client, taskID, actionFQN, jobID string) error {
	if f.HeartbeatFn != nil {
		return f.HeartbeatFn(ctx, client, taskID, actionFQN, jobID)
	}
	return nil
}
