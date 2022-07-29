// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// InitStatus initialize a status builder
func InitStatus(sources *sources.LogSources) {
	var isRunning = atomic.NewBool(true)
	endpoints, _ := config.BuildEndpoints(config.HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	Init(isRunning, endpoints, sources, metrics.LogsExpvars)
}
