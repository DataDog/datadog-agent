// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

// LogSources stores a list of log sources.
type LogSources struct {
	sources []*LogSource
}

// newLogsSource creates a new log sources.
func newLogSources(sources []*LogSource) *LogSources {
	return &LogSources{
		sources: sources,
	}
}

// GetSources returns all the sources currently held.
func (s *LogSources) GetSources() []*LogSource {
	return s.sources
}

// GetValidSources returns all the sources currently held not having errors.
func (s *LogSources) GetValidSources() []*LogSource {
	return s.getSources(func(source *LogSource) bool {
		return !source.Status.IsError()
	})
}

// getSources returns all the sources matching the provided filter.
func (s *LogSources) getSources(filter func(*LogSource) bool) []*LogSource {
	sources := make([]*LogSource, 0)
	for _, source := range s.sources {
		if filter(source) {
			sources = append(sources, source)
		}
	}
	return sources
}
