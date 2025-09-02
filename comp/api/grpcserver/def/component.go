// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package grpcserver defines the component interface for the grpcserver component.
package grpcserver

import (
	"net/http"
)

// team: agent-runtimes

// Component is the component type.
type Component interface {
	BuildServer() http.Handler
}
