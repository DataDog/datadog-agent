// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status fetch information needed to render the 'remote agents' section of the status page
package status

import (
	"embed"
	"io"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

// populateStatus populates the status stats
func populateStatus(registry remoteagentregistry.Component, stats map[string]interface{}) {
	stats["registeredAgents"] = registry.GetRegisteredAgents()
	stats["registeredAgentStatuses"] = registry.GetRegisteredAgentStatuses()
}

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct {
	registry remoteagentregistry.Component
}

// GetProvider returns status.Provider
func GetProvider(registry remoteagentregistry.Component) status.Provider {
	return Provider{registry: registry}
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(p.registry, stats)

	return stats
}

// Name returns the name
func (p Provider) Name() string {
	return "Remote Agents"
}

// Section return the section
func (p Provider) Section() string {
	return "Remote Agents"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(p.registry, stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "remote_agents.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}
