// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package fake provides a fake Docker client to be used in tests.
package fake

import (
	"context"

	"github.com/moby/moby/api/types"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

// SystemAPIClient is a mock
type SystemAPIClient struct {
	InfoFunc func() (system.Info, error)
}

// Events is a mock method
func (c *SystemAPIClient) Events(context.Context, client.EventsListOptions) client.EventsResult {
	return client.EventsResult{}
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
func (c *SystemAPIClient) DiskUsage(context.Context, client.DiskUsageOptions) (client.DiskUsageResult, error) {
	return client.DiskUsageResult{}, nil
}

// Ping is a mock method
func (c *SystemAPIClient) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}
