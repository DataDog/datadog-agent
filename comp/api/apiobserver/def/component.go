// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package apiobserver provides telemetry middleware for api servers
package apiobserver

import (
	"net/http"

	"github.com/gorilla/mux"
)

// team: agent-runtimes

// Component is the component type.
type Component interface {
	Middleware(serverName string, authTagGetter func(r *http.Request) string) mux.MiddlewareFunc
}
