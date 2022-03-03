// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
)

// MockSourceProvider is a fake SourceProvider that can be used to provide fake sources.
//
// This is a useful tool in testing launchers.
type MockSourceProvider struct {
	// SourceChan is the unbuffered channel returned from all source-related methods.
	// Send LogSources to it to trigger behavior from launchers.
	SourceChan chan *logsConfig.LogSource
}

// NewMockSourceProvider creates a new MockSource Provider.
func NewMockSourceProvider() *MockSourceProvider {
	return &MockSourceProvider{
		SourceChan: make(chan *logsConfig.LogSource),
	}
}

// GetAddedForType implements SourceProvider#GetAddedForType.
func (sp *MockSourceProvider) GetAddedForType(sourceType string) chan *config.LogSource {
	return sp.SourceChan
}

// GetRemovedForType implements SourceProvider#GetRemovedForType.
func (sp *MockSourceProvider) GetRemovedForType(sourceType string) chan *config.LogSource {
	return sp.SourceChan
}
