// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcserviceha is a remote config service that can run in the Agent to receive remote config updates from the DD failover DC backend.
package rcserviceha

import (
	"context"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// team: remote-config

// Component is the component type.
type Component interface {
	// ClientGetConfigs is the polling API called by tracers and agents to get the latest configurations
	ClientGetConfigs(_ context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
	// ConfigGetState returns the state of the configuration and the director repos in the local store
	ConfigGetState() (*pbgo.GetStateConfigResponse, error)
}
