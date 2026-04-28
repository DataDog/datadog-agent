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
// For non-container sources (e.g. type:file from conf.d) the calls go through
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
// For container-runtime sources with an identifier, it suppresses the generic
// observer source before forwarding to LogSources.
func (m *adSourceManager) AddSource(src *sources.LogSource) {
	if isContainerSource(src) {
		m.sp.suppressIdentifier(src.Config.Identifier)
	}
	m.logSources.AddSource(src)
}

// RemoveSource implements schedulers.SourceManager.
// For container-runtime sources with an identifier, it releases suppression
// after the source is removed from LogSources.
func (m *adSourceManager) RemoveSource(src *sources.LogSource) {
	m.logSources.RemoveSource(src)
	if isContainerSource(src) {
		m.sp.unsuppressIdentifier(src.Config.Identifier)
	}
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
