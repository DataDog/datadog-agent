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

// team: agent-shared-components

// CollectorSection stores the collector section name
const CollectorSection string = "collector"

// Component interface to access the agent status.
type Component interface {
	// Returns all the agent status information for the format type
	GetStatus(format string, verbose bool) ([]byte, error)
	// Returns only the agent status for the especify section and format type
	GetStatusBySection(section string, format string, verbose bool) ([]byte, error)
}

// Provider interface
type Provider interface {
	// Name is used to sort the status providers alphabetically.
	Name() string
	// Section is used to group the status providers.
	// When displaying the Text output the section is render as a header
	Section() string
	JSON(stats map[string]interface{}) error
	Text(buffer io.Writer) error
	HTML(buffer io.Writer) error
}

// HeaderProvider interface
type HeaderProvider interface {
	// Index is used to choose the order in which the header information is displayed.
	Index() int
	// When displaying the Text output the name is render as a header
	Name() string
	JSON(stats map[string]interface{}) error
	Text(buffer io.Writer) error
	HTML(buffer io.Writer) error
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

type NoopProvider struct{}

func (p NoopProvider) Name() string {
	return ""
}
func (p NoopProvider) Section() string {
	return ""
}
func (p NoopProvider) JSON(_ map[string]interface{}) error {
	return nil
}
func (p NoopProvider) Text(_ io.Writer) error {
	return nil
}
func (p NoopProvider) HTML(_ io.Writer) error {
	return nil
}

type NoopHeaderProvider struct{}

func (p NoopHeaderProvider) Name() string {
	return ""
}
func (p NoopHeaderProvider) Index() int {
	return 0
}
func (p NoopHeaderProvider) JSON(_ map[string]interface{}) error {
	return nil
}
func (p NoopHeaderProvider) Text(_ io.Writer) error {
	return nil
}
func (p NoopHeaderProvider) HTML(_ io.Writer) error {
	return nil
}

// NewInformationProvider returns a InformationProvider to be called when generating the agent status
func NewInformationProvider(provider Provider) InformationProvider {
	return InformationProvider{
		Provider: provider,
	}
}

// NoopInformationProvider returns a Noop InformationProvider
func NoopInformationProvider() InformationProvider {
	return InformationProvider{
		Provider: NoopProvider{},
	}
}

// NewHeaderInformationProvider returns a new HeaderInformationProvider to be called when generating the agent status
func NewHeaderInformationProvider(provider HeaderProvider) HeaderInformationProvider {
	return HeaderInformationProvider{
		Provider: provider,
	}
}

// NoopHeaderInformationProvider returns a Noop HeaderInformationProvider
func NoopHeaderInformationProvider() HeaderInformationProvider {
	return HeaderInformationProvider{
		Provider: NoopHeaderProvider{},
	}
}
