// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package fake provides a fake Docker client to be used in tests.
package fake

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/system"
)

// SystemAPIClient is a mock
type SystemAPIClient struct {
	InfoFunc func() (system.Info, error)
}

// Events is a mock method
func (c *SystemAPIClient) Events(context.Context, types.EventsOptions) (<-chan events.Message, <-chan error) {
	return nil, nil
}

// Info is a mock method
func (c *SystemAPIClient) Info(context.Context) (system.Info, error) {
	return c.InfoFunc()
}

// RegistryLogin is a mock method
func (c *SystemAPIClient) RegistryLogin(context.Context, registry.AuthConfig) (registry.AuthenticateOKBody, error) {
	return registry.AuthenticateOKBody{}, nil
}

// DiskUsage is a mock method
func (c *SystemAPIClient) DiskUsage(context.Context, types.DiskUsageOptions) (types.DiskUsage, error) {
	return types.DiskUsage{}, nil
}

// Ping is a mock method
func (c *SystemAPIClient) Ping(context.Context) (types.Ping, error) {
	return types.Ping{}, nil
}
