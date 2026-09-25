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

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct {
	registry remoteagentregistry.Component
	section  string
}

// GetProvider returns status.Provider
func GetProvider(registry remoteagentregistry.Component) status.Provider {
	return Provider{registry: registry}
}

func (p Provider) getStatusInfo() map[string]interface{} {
	statuses := p.statuses()
	var agents []remoteagentregistry.RegisteredAgent
	if p.section == "" {
		agents = p.registry.GetRegisteredAgents()
	} else {
		for _, remoteStatus := range statuses {
			agents = append(agents, remoteStatus.RegisteredAgent)
		}
	}
	return map[string]interface{}{
		"registeredAgents":        agents,
		"registeredAgentStatuses": statuses,
	}
}

// Name returns the name
func (p Provider) Name() string {
	return "Remote Agents"
}

// Section return the section
func (p Provider) Section() string {
	if p.section != "" {
		return p.section
	}
	return "Remote Agents"
}

// SectionProviders exposes advertised sections through the standard status interface.
func (p Provider) SectionProviders() []status.Provider {
	providers := make([]status.Provider, 0)
	seen := make(map[string]bool)
	for _, agent := range p.registry.GetRegisteredAgents() {
		key := strings.ToLower(agent.StatusSection)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		providers = append(providers, Provider{registry: p.registry, section: agent.StatusSection})
	}

	sort.Slice(providers, func(i, j int) bool {
		return strings.ToLower(providers[i].Section()) < strings.ToLower(providers[j].Section())
	})
	return providers
}

func (p Provider) statuses() []remoteagentregistry.StatusData {
	statuses := p.registry.GetRegisteredAgentStatuses()
	if p.section == "" {
		return statuses
	}
	matching := make([]remoteagentregistry.StatusData, 0, len(statuses))
	for _, remoteStatus := range statuses {
		if strings.EqualFold(remoteStatus.StatusSection, p.section) {
			matching = append(matching, remoteStatus)
		}
	}
	return matching
}

func addJSONField(stats map[string]interface{}, key string, value interface{}) error {
	if _, exists := stats[key]; exists {
		return fmt.Errorf("duplicate remote status JSON key %q", key)
	}
	stats[key] = value
	return nil
}

func mergeJSONPayloads(stats map[string]interface{}, statuses []remoteagentregistry.StatusData) error {
	var errs []error
	for _, remoteStatus := range statuses {
		if remoteStatus.FailureReason != "" {
			errs = append(errs, fmt.Errorf("%s: %s", remoteStatus.DisplayName, remoteStatus.FailureReason))
		}
		if remoteStatus.JSONError != "" {
			errs = append(errs, errors.New(remoteStatus.JSONError))
		}

		keys := make([]string, 0, len(remoteStatus.JSONPayload))
		for key := range remoteStatus.JSONPayload {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if key == "errors" || key == "registeredAgents" || key == "registeredAgentStatuses" {
				errs = append(errs, fmt.Errorf("reserved remote status JSON key %q", key))
				continue
			}
			if err := addJSONField(stats, key, remoteStatus.JSONPayload[key]); err != nil {
				errs = append(errs, err)
			}
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
	statuses := p.statuses()
	var err error
	if p.section == "" {
		err = errors.Join(
			addJSONField(stats, "registeredAgents", p.registry.GetRegisteredAgents()),
			addJSONField(stats, "registeredAgentStatuses", statuses),
		)
	}
	return errors.Join(err, mergeJSONPayloads(stats, statuses))
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	if p.section != "" {
		return renderTextStatuses(buffer, p.statuses())
	}
	return status.RenderText(templatesFS, "remote_agents.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "remote_agents_html.tmpl", buffer, p.getStatusInfo())
}
