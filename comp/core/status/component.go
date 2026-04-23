// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package status displays information about the agent.
package status

// team: agent-configuration

import (
	statusdef "github.com/DataDog/datadog-agent/comp/core/status/def"
)

// CollectorSection stores the collector section name
const CollectorSection = statusdef.CollectorSection

// Component is the component interface. See def/component.go for documentation.
type Component = statusdef.Component

// Params store configurable options for the status component.
type Params = statusdef.Params

// Provider is the provider interface. See def/component.go for documentation.
type Provider = statusdef.Provider

// HeaderProvider is the header provider interface. See def/component.go for documentation.
type HeaderProvider = statusdef.HeaderProvider

// InformationProvider stores the Provider instance.
type InformationProvider = statusdef.InformationProvider

// HeaderInformationProvider stores the HeaderProvider instance.
type HeaderInformationProvider = statusdef.HeaderInformationProvider

// NewInformationProvider returns a InformationProvider to be called when generating the agent status.
func NewInformationProvider(provider Provider) InformationProvider {
	return statusdef.NewInformationProvider(provider)
}

// NewHeaderInformationProvider returns a new HeaderInformationProvider to be called when generating the agent status.
func NewHeaderInformationProvider(provider HeaderProvider) HeaderInformationProvider {
	return statusdef.NewHeaderInformationProvider(provider)
}

// Mock implements mock-specific methods.
type Mock = statusdef.Mock
