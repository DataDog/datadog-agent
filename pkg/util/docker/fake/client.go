// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package fake

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/registry"
)

type FakeSystemAPIClient struct {
	InfoFunc func() (types.Info, error)
}

func (c *FakeSystemAPIClient) Events(ctx context.Context, options types.EventsOptions) (<-chan events.Message, <-chan error) {
	return nil, nil
}

func (c *FakeSystemAPIClient) Info(ctx context.Context) (types.Info, error) {
	return c.InfoFunc()
}

func (c *FakeSystemAPIClient) RegistryLogin(ctx context.Context, auth types.AuthConfig) (registry.AuthenticateOKBody, error) {
	return registry.AuthenticateOKBody{}, nil
}

func (c *FakeSystemAPIClient) DiskUsage(ctx context.Context) (types.DiskUsage, error) {
	return types.DiskUsage{}, nil
}

func (c *FakeSystemAPIClient) Ping(ctx context.Context) (types.Ping, error) {
	return types.Ping{}, nil

}
