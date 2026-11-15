// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmconnectivitycheck provides the interactive NDM connectivity check.
package ndmconnectivitycheck

// team: network-device-monitoring-core

import (
	"net/http"
)

// Component is the component type.
type Component interface {
	ConnectivityCheckEndpointHandler() http.HandlerFunc
}
