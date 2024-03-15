// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import "github.com/DataDog/datadog-agent/pkg/logs/sources"

// ConsumeSources ensures that another component is consuming the channel to prevent
// the producer to get stuck.
func consumeSources(sources *sources.LogSources) {
	go func() {
		sources := sources.GetAddedForType("foo")
		//nolint:revive // TODO(AML) Fix revive linter
		for range sources {
		}
	}()
}

// CreateSources creates sources
func CreateSources(sourcesArray []*sources.LogSource) *sources.LogSources {
	logSources := sources.NewLogSources()
	for _, source := range sourcesArray {
		logSources.AddSource(source)
	}
	consumeSources(logSources)
	return logSources
}
