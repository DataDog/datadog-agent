// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schedulers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// sourceManager implements the SourceManager interface.
//
// NOTE: when support for services is removed, this struct will be unnecessary,
// as config.Sources will satisfy the interface.
type sourceManager struct {
	sources  *sources.LogSources
	services *service.Services
}

var _ SourceManager = &sourceManager{}

// AddSource implements SourceManager#AddSource.
func (sm *sourceManager) AddSource(source *sources.LogSource) {
	sm.sources.AddSource(source)
}

// RemoveSource implements SourceManager#RemoveSource.
func (sm *sourceManager) RemoveSource(source *sources.LogSource) {
	panic("not called")
}

// GetSources implements SourceManager#GetSources.
func (sm *sourceManager) GetSources() []*sources.LogSource {
	panic("not called")
}

// AddService implements SourceManager#AddService.
func (sm *sourceManager) AddService(service *service.Service) {
	panic("not called")
}

// RemoveService implements SourceManager#RemoveService.
func (sm *sourceManager) RemoveService(service *service.Service) {
	panic("not called")
}

// MockAddRemove is an event observed by MockSourceManager
type MockAddRemove struct {
	// Added is true if this source was added; otherwise it was removed
	Add bool

	// Source is the source that was added or removed, or nil.
	Source *sources.LogSource

	// Service is the service that was added or removed, or nil.
	Service *service.Service
}

// MockSourceManager is a "spy" that records the AddSource and RemoveSource
// calls that it receives.
//
// This is a useful tool in testing schedulers.  Its zero value is a valid
// beginning state.
type MockSourceManager struct {
	// Events are the events that occurred in the spy
	Events []MockAddRemove

	// Sources are the sources returned by GetSources
	Sources []*sources.LogSource
}

var _ SourceManager = &MockSourceManager{}

// AddSource implements SourceManager#AddSource.
func (sm *MockSourceManager) AddSource(source *sources.LogSource) {
	panic("not called")
}

// RemoveSource implements SourceManager#RemoveSource.
func (sm *MockSourceManager) RemoveSource(source *sources.LogSource) {
	panic("not called")
}

// GetSources implements SourceManager#GetSources.
func (sm *MockSourceManager) GetSources() []*sources.LogSource {
	panic("not called")
}

// AddService implements SourceManager#AddService.
func (sm *MockSourceManager) AddService(service *service.Service) {
	panic("not called")
}

// RemoveService implements SourceManager#RemoveService.
func (sm *MockSourceManager) RemoveService(service *service.Service) {
	panic("not called")
}
