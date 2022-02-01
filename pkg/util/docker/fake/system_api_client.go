// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package fake

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/registry"
)

type SystemAPIClient struct {
	InfoFunc func() (types.Info, error)
}

func (c *SystemAPIClient) Events(ctx context.Context, options types.EventsOptions) (<-chan events.Message, <-chan error) {
	return nil, nil
}

func (c *SystemAPIClient) Info(ctx context.Context) (types.Info, error) {
	return c.InfoFunc()
}

func (c *SystemAPIClient) RegistryLogin(ctx context.Context, auth types.AuthConfig) (registry.AuthenticateOKBody, error) {
	return registry.AuthenticateOKBody{}, nil
}

func (c *SystemAPIClient) DiskUsage(ctx context.Context) (types.DiskUsage, error) {
	return types.DiskUsage{}, nil
}

func (c *SystemAPIClient) Ping(ctx context.Context) (types.Ping, error) {
	return types.Ping{}, nil
}
