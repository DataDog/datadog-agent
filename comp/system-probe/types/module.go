// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package types provides common types for system-probe module components
package types

import (
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// ErrNotEnabled is a special error type that should be returned by a Factory
// when the associated Module is not enabled.
var ErrNotEnabled = errors.New("module is not enabled")

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

// ModuleOrder is the desired creation order for system-probe modules
var ModuleOrder = []types.ModuleName{
	config.EBPFModule,
	config.NetworkTracerModule,
	config.TCPQueueLengthTracerModule,
	config.OOMKillProbeModule,
	config.EventMonitorModule, // there is a dependency from EventMonitor -> NetworkTracer, so EventMonitor has to follow NetworkTracer
	config.ProcessModule,
	config.DynamicInstrumentationModule, // dynamic instrumentation needs to be after EventMonitor
	config.LanguageDetectionModule,
	config.ComplianceModule,
	config.PingModule,
	config.TracerouteModule,
	config.DiscoveryModule,
	config.GPUMonitoringModule, // GPU monitoring needs to be initialized after EventMonitor, so that we have the event consumer ready
	config.SoftwareInventoryModule,
	config.PrivilegedLogsModule,
	config.WindowsCrashDetectModule,
}
