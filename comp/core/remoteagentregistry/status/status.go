// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status fetch information needed to render the 'remote agents' section of the status page
package status

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

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

// Sections returns the status sections advertised by registered remote agents.
func (p Provider) Sections() []string {
	sections := make([]string, 0)
	for _, agent := range p.registry.GetRegisteredAgents() {
		if agent.StatusSection == "" {
			continue
		}

		seen := false
		for _, section := range sections {
			if strings.EqualFold(section, agent.StatusSection) {
				seen = true
				break
			}
		}
		if !seen {
			sections = append(sections, agent.StatusSection)
		}
	}

	sort.Slice(sections, func(i, j int) bool {
		return strings.ToLower(sections[i]) < strings.ToLower(sections[j])
	})
	return sections
}

func (p Provider) statusesForSection(section string) []remoteagentregistry.StatusData {
	statuses := p.registry.GetRegisteredAgentStatuses()
	matching := make([]remoteagentregistry.StatusData, 0, len(statuses))
	for _, remoteStatus := range statuses {
		if strings.EqualFold(remoteStatus.StatusSection, section) {
			matching = append(matching, remoteStatus)
		}
	}
	return matching
}

func mergeJSONPayloads(stats map[string]interface{}, statuses []remoteagentregistry.StatusData) error {
	var errs []error
	for _, remoteStatus := range statuses {
		if remoteStatus.JSONError != "" {
			errs = append(errs, errors.New(remoteStatus.JSONError))
		}

		keys := make([]string, 0, len(remoteStatus.JSONPayload))
		for key := range remoteStatus.JSONPayload {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if _, exists := stats[key]; exists {
				errs = append(errs, fmt.Errorf("duplicate remote status JSON key %q", key))
				continue
			}
			stats[key] = remoteStatus.JSONPayload[key]
		}
	}
	return errors.Join(errs...)
}

func renderTextStatuses(buffer io.Writer, statuses []remoteagentregistry.StatusData) error {
	for _, remoteStatus := range statuses {
		if remoteStatus.FailureReason != "" {
			if _, err := io.WriteString(buffer, remoteStatus.FailureReason); err != nil {
				return err
			}
			continue
		}

		if details, ok := remoteStatus.NamedSections["Details"]; ok {
			if rawStatus, ok := details[""]; ok {
				if _, err := io.WriteString(buffer, rawStatus); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	statuses := p.registry.GetRegisteredAgentStatuses()
	stats["registeredAgents"] = p.registry.GetRegisteredAgents()
	stats["registeredAgentStatuses"] = statuses

	return mergeJSONPayloads(stats, statuses)
}

// JSONBySection populates the status map from matching remote agents.
func (p Provider) JSONBySection(section string, _ bool, stats map[string]interface{}) error {
	return mergeJSONPayloads(stats, p.statusesForSection(section))
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "remote_agents.tmpl", buffer, p.getStatusInfo())
}

// TextBySection renders text status from matching remote agents.
func (p Provider) TextBySection(section string, _ bool, buffer io.Writer) error {
	return renderTextStatuses(buffer, p.statusesForSection(section))
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "remote_agents_html.tmpl", buffer, p.getStatusInfo())
}

// HTMLBySection renders HTML status from matching remote agents.
func (p Provider) HTMLBySection(section string, _ bool, buffer io.Writer) error {
	statuses := p.statusesForSection(section)
	agents := make([]remoteagentregistry.RegisteredAgent, 0, len(statuses))
	for _, remoteStatus := range statuses {
		agents = append(agents, remoteStatus.RegisteredAgent)
	}

	stats := map[string]interface{}{
		"registeredAgents":        agents,
		"registeredAgentStatuses": statuses,
	}
	return status.RenderHTML(templatesFS, "remote_agents_html.tmpl", buffer, stats)
}
