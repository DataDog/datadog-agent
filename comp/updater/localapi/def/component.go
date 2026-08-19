// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localapi is the updater local api component.
package localapi

import (
	"context"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// team: fleet windows-products

// Component is the interface for the updater local api component.
type Component interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// StatusEndpoint is the local API route that reports installer state.
const StatusEndpoint = "/status"

// StatusResponse is the response to the status endpoint.
type StatusResponse struct {
	APIResponse
	RemoteConfigState []*pbgo.PackageState `json:"remote_config_state"`
	SecretsPubKey     string               `json:"secrets_pub_key"`
}

// APIResponse is the response to a local API request.
type APIResponse struct {
	Error *APIError `json:"error,omitempty"`
}

// APIError is an error returned by the local API.
type APIError struct {
	Message string `json:"message"`
}
