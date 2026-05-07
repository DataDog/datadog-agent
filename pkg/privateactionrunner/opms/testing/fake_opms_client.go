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
	HealthCheckFn func(ctx context.Context) (*opms.HealthCheckData, error)
	DequeueTaskFn func(ctx context.Context) (*types.Task, time.Duration, error)
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

func (f *FakeOpmsClient) PublishSuccess(_ context.Context, _ actionsclientpb.Client, _, _, _ string, _ interface{}, _ string) error {
	return nil
}

func (f *FakeOpmsClient) PublishFailure(_ context.Context, _ actionsclientpb.Client, _, _, _ string, _ aperrorpb.ActionPlatformErrorCode, _, _ string) error {
	return nil
}

func (f *FakeOpmsClient) Heartbeat(_ context.Context, _ actionsclientpb.Client, _, _, _ string) error {
	return nil
}
