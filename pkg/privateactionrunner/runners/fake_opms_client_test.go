// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

// fakeOpmsClient is a configurable opms.Client for use in runner unit tests.
// Set the function fields to override individual methods; unset fields return
// safe zero-value defaults.
type fakeOpmsClient struct {
	healthCheckFn func(ctx context.Context) (*opms.HealthCheckData, error)
	dequeueTaskFn func(ctx context.Context) (*types.Task, time.Duration, error)
}

func (f *fakeOpmsClient) HealthCheck(ctx context.Context) (*opms.HealthCheckData, error) {
	if f.healthCheckFn != nil {
		return f.healthCheckFn(ctx)
	}
	return &opms.HealthCheckData{}, nil
}

func (f *fakeOpmsClient) DequeueTask(ctx context.Context) (*types.Task, time.Duration, error) {
	if f.dequeueTaskFn != nil {
		return f.dequeueTaskFn(ctx)
	}
	return nil, 0, nil
}

func (f *fakeOpmsClient) PublishSuccess(_ context.Context, _ actionsclientpb.Client, _, _, _ string, _ interface{}, _ string) error {
	return nil
}

func (f *fakeOpmsClient) PublishFailure(_ context.Context, _ actionsclientpb.Client, _, _, _ string, _ aperrorpb.ActionPlatformErrorCode, _, _ string) error {
	return nil
}

func (f *fakeOpmsClient) Heartbeat(_ context.Context, _ actionsclientpb.Client, _, _, _ string) error {
	return nil
}
