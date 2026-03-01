// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the remoteagentregistry component
package mock

import (
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

// Component is a configurable mock for the remoteagentregistry component.
type Component struct {
	Statuses []remoteagentregistry.StatusData
}

var _ remoteagentregistry.Component = (*Component)(nil)

func (m *Component) RegisterRemoteAgent(_ *remoteagentregistry.RegistrationData) (string, uint32, error) {
	return "", 0, nil
}

func (m *Component) RefreshRemoteAgent(_ string) bool { return false }

func (m *Component) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent { return nil }

func (m *Component) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData { return m.Statuses }

func (m *Component) GetStatusByFlavor(flavor string) (remoteagentregistry.StatusData, bool) {
	for _, s := range m.Statuses {
		if s.Flavor == flavor {
			return s, true
		}
	}
	return remoteagentregistry.StatusData{}, false
}
