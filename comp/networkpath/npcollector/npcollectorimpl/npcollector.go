// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npcollectorimpl implements network path collector
package npcollectorimpl

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
)

type npCollectorImpl struct {
	epForwarder      eventplatform.Component
	collectorConfigs *collectorConfigs
}

func (s *npCollectorImpl) ScheduleConns(_ []*model.Connection) {
	if !s.collectorConfigs.connectionsMonitoringEnabled {
		return
	}
	// TODO: IMPLEMENTATION IN SEPARATE PR (to make PRs easier to review)
}

func newNoopNpCollectorImpl() *npCollectorImpl {
	return &npCollectorImpl{
		collectorConfigs: &collectorConfigs{},
	}
}

func newNpCollectorImpl(epForwarder eventplatform.Component, collectorConfigs *collectorConfigs) *npCollectorImpl {
	return &npCollectorImpl{
		epForwarder:      epForwarder,
		collectorConfigs: collectorConfigs,
	}
}
