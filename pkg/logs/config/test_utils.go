// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

// ConsumeSources ensures that another component is consuming the channel to prevent
// the producer to get stuck.
func consumeSources(sources *LogSources) {
	go func() {
		sources := sources.GetAddedForType("foo")
		for range sources {
		}
	}()
}

// CreateSources creates sources
func CreateSources(sourcesArray []*LogSource) *LogSources {
	logSources := NewLogSources()
	for _, source := range sourcesArray {
		logSources.AddSource(source)
	}
	consumeSources(logSources)
	return logSources
}
