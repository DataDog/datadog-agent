// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schedulers

import (
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// sourceManager implements SourceManager.
//
// NOTE: when support for services is removed, this struct will be unnecessary,
// as config.Sources will satisfy the interface.
type sourceManager struct {
	sources  *logsConfig.LogSources
	services *service.Services
}

// AddSource implements SourceManager#AddSource.
func (sm *sourceManager) AddSource(source *logsConfig.LogSource) {
	sm.sources.AddSource(source)
}

// RemoveSource implements SourceManager#RemoveSource.
func (sm *sourceManager) RemoveSource(source *logsConfig.LogSource) {
	sm.sources.RemoveSource(source)
}

// AddService implements SourceManager#AddService.
func (sm *sourceManager) AddService(service *service.Service) {
	sm.services.AddService(service)
}

// RemoveService implements SourceManager#RemoveService.
func (sm *sourceManager) RemoveService(service *service.Service) {
	sm.services.RemoveService(service)
}

// SpyAddRemove is an event observed by SourceManagerSpy
type SpyAddRemove struct {
	// Added is true if this source was added; otherwise it was removed
	Add bool

	// Source is the source that was added or removed, or nil.
	Source *logsConfig.LogSource

	// Service is the service that was added or removed, or nil.
	Service *service.Service
}

// SourceManagerSpy is a "spy" that records the AddSource and RemoveSource
// calls that it receives.
//
// This is a useful tool in testing schedulers.  Its zero value is a valid
// beginning state.
type SourceManagerSpy struct {
	// Events are the events that occurred in the spy
	Events []SpyAddRemove
}

// AddSource implements SourceManager#AddSource.
func (sm *SourceManagerSpy) AddSource(source *logsConfig.LogSource) {
	sm.Events = append(sm.Events, SpyAddRemove{Add: true, Source: source})
}

// RemoveSource implements SourceManager#RemoveSource.
func (sm *SourceManagerSpy) RemoveSource(source *logsConfig.LogSource) {
	sm.Events = append(sm.Events, SpyAddRemove{Add: false, Source: source})
}

// AddService implements SourceManager#AddService.
func (sm *SourceManagerSpy) AddService(service *service.Service) {
	sm.Events = append(sm.Events, SpyAddRemove{Add: true, Service: service})
}

// RemoveService implements SourceManager#RemoveService.
func (sm *SourceManagerSpy) RemoveService(service *service.Service) {
	sm.Events = append(sm.Events, SpyAddRemove{Add: false, Service: service})
}
