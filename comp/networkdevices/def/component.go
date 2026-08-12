// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package networkdevices provides the Agent-side component for NDM.
package networkdevices

// team: network-device-monitoring-core

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// Component is the component type.
type Component interface {
	CheckConnectivity(ctx context.Context, req connectivity.Request) (connectivity.Result, error)
	ConnectivityCheckEndpointHandler() http.HandlerFunc
}
