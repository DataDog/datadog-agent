// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package modules

import (
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// registerUSMEndpoints is a stub for Darwin
// Universal Service Monitoring (USM) is not yet implemented on macOS
func registerUSMEndpoints(nt *networkTracer, httpMux *module.Router) {
	// USM features (HTTP, HTTP/2, Kafka monitoring) not implemented on Darwin yet
	// This is a no-op stub to allow compilation
}
