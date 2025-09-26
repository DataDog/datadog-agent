// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package hostprofiler implements the "host-profiler" bundle,
package hostprofiler

import (
	collectorfx "github.com/DataDog/datadog-agent/comp/host-profiler/collector/fx"
	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry-agent profiling-full-host

// Bundle defines the fx options for this bundle.
func Bundle(params collectorimpl.Params) fxutil.BundleOptions {
	return fxutil.Bundle(
		collectorfx.Module(params), // This is the main component for the host profiler
	)
}
