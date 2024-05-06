// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npschedulerimpl implements the scheduler for network path
package npschedulerimpl

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
)

type npSchedulerImpl struct {
	epForwarder eventplatform.Component
	enabled     bool
}

func (s *npSchedulerImpl) Schedule(_ string, _ uint16) error {
	// TODO: IMPLEMENTATION IN SEPARATE PR (to make PRs easier to review)
	return nil
}

func (s *npSchedulerImpl) Enabled() bool {
	return s.enabled
}

func newNoopNpSchedulerImpl() *npSchedulerImpl {
	return &npSchedulerImpl{
		enabled: false,
	}
}

func newNpSchedulerImpl(epForwarder eventplatform.Component) *npSchedulerImpl {
	return &npSchedulerImpl{
		enabled:     true,
		epForwarder: epForwarder,
	}
}
