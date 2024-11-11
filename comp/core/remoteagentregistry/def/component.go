// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistry provides an integration point for remote agents to register and be able to report their
// status and emit flare data
package remoteagentregistry

// team: agent-shared-components

// Component is the component type.
type Component interface {
	RegisterRemoteAgent(req *RegistrationData) (uint32, error)
	GetRegisteredAgents() []*RegisteredAgent
	GetRegisteredAgentStatuses() []*StatusData
}
