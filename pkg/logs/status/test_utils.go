// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package status

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// ConsumeSources ensures that another component is consuming the channel to prevent
// the producer to get stuck.
func ConsumeSources(sources *config.LogSources) {
	go func() {
		sources := sources.GetAddedForType("foo")
		for range sources {
		}
	}()
}

// CreateSources creates sources and initialize a status builder
func CreateSources(sourcesArray []*config.LogSource) *config.LogSources {
	logSources := config.NewLogSources()
	for _, source := range sourcesArray {
		logSources.AddSource(source)
	}
	ConsumeSources(logSources)
	var isRunning int32 = 1
	Init(&isRunning, logSources, metrics.LogsExpvars)

	return logSources
}
