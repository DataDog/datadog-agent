// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcservice is a remote config service that can run within the agent to receive remote config updates from the DD backend.
package rcservice

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// team: remote-config

// Params is the set of parameters required for the remote config service.
type Params struct {
	// Options is the set of options for the remote config service.
	Options []service.Option
}

// Component is the component type.
type Component interface {
	// ClientGetConfigs is the polling API called by tracers and agents to get the latest configurations
	ClientGetConfigs(_ context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
	// ConfigGetState returns the state of the configuration and the director repos in the local store
	ConfigGetState() (*pbgo.GetStateConfigResponse, error)
	// ConfigResetState resets the remote configuration state, clearing the local store and reinitializing the uptane client
	ConfigResetState() (*pbgo.ResetStateConfigResponse, error)
}
