// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// MockSourceProvider is a fake SourceProvider that can be used to provide fake sources.
//
// This is a useful tool in testing launchers.
type MockSourceProvider struct {
	// SourceChan is the unbuffered channel returned from all source-related methods.
	// Send LogSources to it to trigger behavior from launchers.
	SourceChan chan *sources.LogSource
}

// NewMockSourceProvider creates a new MockSource Provider.
func NewMockSourceProvider() *MockSourceProvider {
	return &MockSourceProvider{
		SourceChan: make(chan *sources.LogSource),
	}
}

// SubscribeAll implements SourceProvider#SubscribeAll.
func (sp *MockSourceProvider) SubscribeAll() (chan *sources.LogSource, chan *sources.LogSource) {
	return sp.SourceChan, sp.SourceChan
}

// SubscribeForType implements SourceProvider#SubscribeForType.
func (sp *MockSourceProvider) SubscribeForType(sourceType string) (chan *sources.LogSource, chan *sources.LogSource) {
	return sp.SourceChan, sp.SourceChan
}

// GetAddedForType implements SourceProvider#GetAddedForType.
func (sp *MockSourceProvider) GetAddedForType(sourceType string) chan *sources.LogSource {
	return sp.SourceChan
}
