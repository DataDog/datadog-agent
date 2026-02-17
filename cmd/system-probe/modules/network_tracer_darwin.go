// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package modules

import (
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

func init() { registerModule(NetworkTracer) }

// NetworkTracer is a factory for NPM's tracer on Darwin
var NetworkTracer = &module.Factory{
	Name:             config.NetworkTracerModule,
	ConfigNamespaces: networkTracerModuleConfigNamespaces,
	Fn:               createNetworkTracerModule,
}

// platformRegister is a stub for Darwin
// Platform-specific endpoints (like network_id on Linux) are not implemented yet
func (nt *networkTracer) platformRegister(_ *module.Router) error {
	// No platform-specific endpoints for Darwin yet
	return nil
}
