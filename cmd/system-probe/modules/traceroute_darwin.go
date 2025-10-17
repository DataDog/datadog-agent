// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package modules

import (
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// Traceroute is a factory for NDMs Traceroute module
var Traceroute = &module.Factory{
	Name:             config.TracerouteModule,
	ConfigNamespaces: tracerouteConfigNamespaces,
	Fn:               createTracerouteModule,
}

// startPlatformDriver is a no-op on Darwin
func startPlatformDriver() error {
	// No driver needed on Darwin
	return nil
}

// stopPlatformDriver is a no-op on darwin
func stopPlatformDriver() error {
	// No driver needed on Darwin
	return nil
}
