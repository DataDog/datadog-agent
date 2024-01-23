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
	panic("not called")
}

// SubscribeAll implements SourceProvider#SubscribeAll.
func (sp *MockSourceProvider) SubscribeAll() (chan *sources.LogSource, chan *sources.LogSource) {
	panic("not called")
}

// SubscribeForType implements SourceProvider#SubscribeForType.
//
//nolint:revive // TODO(AML) Fix revive linter
func (sp *MockSourceProvider) SubscribeForType(sourceType string) (chan *sources.LogSource, chan *sources.LogSource) {
	panic("not called")
}

// GetAddedForType implements SourceProvider#GetAddedForType.
//
//nolint:revive // TODO(AML) Fix revive linter
func (sp *MockSourceProvider) GetAddedForType(sourceType string) chan *sources.LogSource {
	panic("not called")
}
