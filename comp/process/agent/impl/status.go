// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package agentimpl implements a component for the process agent.
package agentimpl

import (
	"embed"
	"encoding/json"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	processStatus "github.com/DataDog/datadog-agent/pkg/process/util/status"
)

// StatusProvider is the type for process component status methods.
// It renders the "Process Component" section shown by `agent status` when
// the process functionality runs embedded in the core agent. Data is read
// directly from the in-process state populated by InitExpvars at startup,
// not over HTTP.
type StatusProvider struct{}

// NewStatusProvider returns a new StatusProvider for the embedded process
// component status section.
func NewStatusProvider() *StatusProvider {
	return &StatusProvider{}
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (s StatusProvider) Name() string {
	return "Process Component"
}

// Section return the section
func (s StatusProvider) Section() string {
	return "Process Component"
}

func (s StatusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	values := s.populateStatus()

	stats["processComponentStatus"] = values

	return stats
}

// populateStatus reads the process component's status directly from the
// in-process state populated by InitExpvars and returns it as a generic map
// for template rendering. No HTTP, no expvar parsing.
func (s StatusProvider) populateStatus() map[string]interface{} {
	agentStatus := processStatus.GetInProcessStatus()

	bytes, err := json.Marshal(agentStatus)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	result := make(map[string]interface{})
	if err := json.Unmarshal(bytes, &result); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return result
}

// JSON populates the status map
func (s StatusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values := s.populateStatus()

	stats["processComponentStatus"] = values

	return nil
}

// Text renders the text output
func (s StatusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "processcomponent.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s StatusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
