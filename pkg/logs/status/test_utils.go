// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package status

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// InitStatus initialize a status builder
func InitStatus(sources *config.LogSources) {
	var isRunning int32 = 1
	Init(&isRunning, sources, metrics.LogsExpvars)
}
