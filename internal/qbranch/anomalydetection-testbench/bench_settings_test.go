// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
)

// benchmarkSettings is the ComponentSettings used by all benchmarks.
// Defaults to empty (uses catalog defaults). It can be overridden by calling
// applyBenchmarkOnlyFlag from TestMain.
var benchmarkSettings observerimpl.ComponentSettings
