// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// adSourceManager is a schedulers.SourceManager that intercepts container-type
// AD source additions and removals to coordinate suppression of generic
// observer container sources via sourceProvider.
//
// For non-container sources (e.g. type:file from conf.d) the calls pass through
// unchanged — suppression is only applied when source.Config.Identifier is set
// and the source type is a known container runtime.
type adSourceManager struct {
	logSources *sources.LogSources
	services   *service.Services
	sp         *sourceProvider
}

var _ schedulers.SourceManager = (*adSourceManager)(nil)

func newADSourceManager(logSources *sources.LogSources, services *service.Services, sp *sourceProvider) *adSourceManager {
	return &adSourceManager{
		logSources: logSources,
		services:   services,
		sp:         sp,
	}
}

// AddSource implements schedulers.SourceManager.
// Agent container sources are dropped so AD/CCA cannot bypass the internal log
// tap gate. Other container-runtime sources are added to LogSources before
// suppression evicts any generic source, eliminating the TOCTOU window where
// neither source would be active.
func (m *adSourceManager) AddSource(src *sources.LogSource) {
	if isContainerSource(src) && m.sp.isAgentContainerID(src.Config.Identifier) {
		return
	}
	m.logSources.AddSource(src)
	if isContainerSource(src) {
		m.sp.suppressIdentifier(src.Config.Identifier)
	}
}

// RemoveSource implements schedulers.SourceManager.
// For container-runtime sources with an identifier, it releases suppression
// before removing from LogSources so that a concurrent workloadmeta Set event
// can immediately re-create the generic source without waiting for another event.
func (m *adSourceManager) RemoveSource(src *sources.LogSource) {
	if isContainerSource(src) {
		m.sp.unsuppressIdentifier(src.Config.Identifier)
	}
	m.logSources.RemoveSource(src)
}

// GetSources implements schedulers.SourceManager.
func (m *adSourceManager) GetSources() []*sources.LogSource {
	return m.logSources.GetSources()
}

// AddService implements schedulers.SourceManager.
func (m *adSourceManager) AddService(svc *service.Service) {
	m.services.AddService(svc)
}

// RemoveService implements schedulers.SourceManager.
func (m *adSourceManager) RemoveService(svc *service.Service) {
	m.services.RemoveService(svc)
}

// isContainerSource returns true when the source was produced by a container
// AD config — i.e. it has a container-runtime type and a non-empty identifier.
func isContainerSource(src *sources.LogSource) bool {
	if src.Config.Identifier == "" {
		return false
	}
	switch src.Config.Type {
	case logsconfig.DockerType, logsconfig.ContainerdType:
		return true
	}
	return false
}
