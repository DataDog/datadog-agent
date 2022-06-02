// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schedulers

import (
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// sourceManager implements the SourceManager interface.
//
// NOTE: when support for services is removed, this struct will be unnecessary,
// as config.Sources will satisfy the interface.
type sourceManager struct {
	sources  *logsConfig.LogSources
	services *service.Services
}

var _ SourceManager = &sourceManager{}

// AddSource implements SourceManager#AddSource.
func (sm *sourceManager) AddSource(source *logsConfig.LogSource) {
	sm.sources.AddSource(source)
}

// RemoveSource implements SourceManager#RemoveSource.
func (sm *sourceManager) RemoveSource(source *logsConfig.LogSource) {
	sm.sources.RemoveSource(source)
}

// GetSources implements SourceManager#GetSources.
func (sm *sourceManager) GetSources() []*logsConfig.LogSource {
	return sm.sources.GetSources()
}

// AddService implements SourceManager#AddService.
func (sm *sourceManager) AddService(service *service.Service) {
	sm.services.AddService(service)
}

// RemoveService implements SourceManager#RemoveService.
func (sm *sourceManager) RemoveService(service *service.Service) {
	sm.services.RemoveService(service)
}

// MockAddRemove is an event observed by MockSourceManager
type MockAddRemove struct {
	// Added is true if this source was added; otherwise it was removed
	Add bool

	// Source is the source that was added or removed, or nil.
	Source *logsConfig.LogSource

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
	Sources []*logsConfig.LogSource
}

var _ SourceManager = &MockSourceManager{}

// AddSource implements SourceManager#AddSource.
func (sm *MockSourceManager) AddSource(source *logsConfig.LogSource) {
	sm.Events = append(sm.Events, MockAddRemove{Add: true, Source: source})
}

// RemoveSource implements SourceManager#RemoveSource.
func (sm *MockSourceManager) RemoveSource(source *logsConfig.LogSource) {
	sm.Events = append(sm.Events, MockAddRemove{Add: false, Source: source})
}

// GetSources implements SourceManager#GetSources.
func (sm *MockSourceManager) GetSources() []*logsConfig.LogSource {
	sources := make([]*logsConfig.LogSource, len(sm.Sources))
	copy(sources, sm.Sources)
	return sources
}

// AddService implements SourceManager#AddService.
func (sm *MockSourceManager) AddService(service *service.Service) {
	sm.Events = append(sm.Events, MockAddRemove{Add: true, Service: service})
}

// RemoveService implements SourceManager#RemoveService.
func (sm *MockSourceManager) RemoveService(service *service.Service) {
	sm.Events = append(sm.Events, MockAddRemove{Add: false, Service: service})
}
