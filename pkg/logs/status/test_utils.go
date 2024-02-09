// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package status

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

// InitStatus initialize a status builder
func InitStatus(coreConfig pkgConfig.Reader, sources *sources.LogSources) {
	var isRunning = atomic.NewBool(true)
	tracker := tailers.NewTailerTracker()
	endpoints, _ := config.BuildEndpoints(coreConfig, config.HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	Init(isRunning, endpoints, sources, tracker, metrics.LogsExpvars)
}
