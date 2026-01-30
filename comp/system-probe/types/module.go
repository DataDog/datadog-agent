// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/fx"
)

type SystemProbeModule interface {
	GetStats() map[string]interface{}
	Register(SystemProbeRouter) error
	Close()
}

type SystemProbeRouter interface {
	HandleFunc(path string, responseWriter func(http.ResponseWriter, *http.Request)) *mux.Route
	Unregister()
}

type ProvidesSystemProbeModule struct {
	fx.Out

	Component SystemProbeModuleComponent `group:"systemprobe_module"`
}
