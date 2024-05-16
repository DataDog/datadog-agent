// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npschedulerimpl implements the scheduler for network path
package npschedulerimpl

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
)

type npSchedulerImpl struct {
	epForwarder      eventplatform.Component
	collectorConfigs *collectorConfigs
}

func (s *npSchedulerImpl) ScheduleConns(_ []*model.Connection) {
	if !s.collectorConfigs.connectionsMonitoringEnabled {
		return
	}
	// TODO: IMPLEMENTATION IN SEPARATE PR (to make PRs easier to review)
}

func newNoopNpSchedulerImpl() *npSchedulerImpl {
	return &npSchedulerImpl{
		collectorConfigs: &collectorConfigs{},
	}
}

func newNpSchedulerImpl(epForwarder eventplatform.Component, collectorConfigs *collectorConfigs) *npSchedulerImpl {
	return &npSchedulerImpl{
		epForwarder:      epForwarder,
		collectorConfigs: collectorConfigs,
	}
}
