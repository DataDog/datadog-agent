// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package status displays information about the agent.
package status

import (
	"io"

	"go.uber.org/fx"
)

// team: agent-configuration

// CollectorSection stores the collector section name
const CollectorSection string = "collector"

// Component interface to access the agent status.
type Component interface {
	// Returns all the agent status information for the format type
	GetStatus(format string, verbose bool, excludeSection ...string) ([]byte, error)
	//
	GetSections() []string
	// Returns only the agent status for the specified sections and format type
	GetStatusBySections(sections []string, format string, verbose bool) ([]byte, error)
}

// Params store configurable options for the status component
type Params struct {
	PythonVersionGetFunc func() string
}

// Provider interface
type Provider interface {
	// Name is used to sort the status providers alphabetically.
	Name() string
	// Section is used to group the status providers.
	// When displaying the Text output the section is render as a header
	Section() string
	JSON(verbose bool, stats map[string]interface{}) error
	Text(verbose bool, buffer io.Writer) error
	HTML(verbose bool, buffer io.Writer) error
}

// HeaderProvider interface
type HeaderProvider interface {
	// Index is used to choose the order in which the header information is displayed.
	Index() int
	// When displaying the Text output the name is render as a header
	Name() string
	JSON(verbose bool, stats map[string]interface{}) error
	Text(verbose bool, buffer io.Writer) error
	HTML(verbose bool, buffer io.Writer) error
}

// InformationProvider stores the Provider instance
type InformationProvider struct {
	fx.Out

	Provider Provider `group:"status"`
}

// HeaderInformationProvider stores the HeaderProvider instance
type HeaderInformationProvider struct {
	fx.Out

	Provider HeaderProvider `group:"header_status"`
}

// NewInformationProvider returns a InformationProvider to be called when generating the agent status
func NewInformationProvider(provider Provider) InformationProvider {
	return InformationProvider{
		Provider: provider,
	}
}

// NewHeaderInformationProvider returns a new HeaderInformationProvider to be called when generating the agent status
func NewHeaderInformationProvider(provider HeaderProvider) HeaderInformationProvider {
	return HeaderInformationProvider{
		Provider: provider,
	}
}
