// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcservice is a remote config service that can run within the agent to receive remote config updates from the DD backend.
package rcservice

import (
	"context"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// team: remote-config

// Component is the component type.
type Component interface {
	// Start starts the remote configuration management service
	Start(ctx context.Context)
	// Stop stops the refresh loop and closes the on-disk DB cache
	Stop() error
	// ClientGetConfigs is the polling API called by tracers and agents to get the latest configurations
	ClientGetConfigs(_ context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
	// ConfigGetState returns the state of the configuration and the director repos in the local store
	ConfigGetState() (*pbgo.GetStateConfigResponse, error)
}
