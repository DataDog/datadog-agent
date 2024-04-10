// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Only use for testing
var logsProvider = logsStatus.Get

// StatusProvider is the type for logs agent status methods
type StatusProvider struct{}

// Name returns the name
func (p StatusProvider) Name() string {
	return "Logs Agent"
}

// Section returns the section
func (p StatusProvider) Section() string {
	return "Logs Agent"
}

func (p StatusProvider) getStatusInfo(verbose bool) map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(verbose, stats)

	return stats
}

func (p StatusProvider) populateStatus(verbose bool, stats map[string]interface{}) {
	stats["logsStats"] = logsProvider(verbose)
}

// JSON populates the status map
func (p StatusProvider) JSON(verbose bool, stats map[string]interface{}) error {
	p.populateStatus(verbose, stats)

	return nil
}

// Text renders the text output
func (p StatusProvider) Text(verbose bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "logsagent.tmpl", buffer, p.getStatusInfo(verbose))
}

// HTML renders the HTML output
func (p StatusProvider) HTML(verbose bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "logsagentHTML.tmpl", buffer, p.getStatusInfo(verbose))
}

// AddGlobalWarning keeps track of a warning message to display on the status.
func (p StatusProvider) AddGlobalWarning(key string, warning string) {
	logsStatus.AddGlobalWarning(key, warning)
}

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
func (p StatusProvider) RemoveGlobalWarning(key string) {
	logsStatus.RemoveGlobalWarning(key)
}

// NewStatusProvider fetches the status and returns a service wrapping it
func NewStatusProvider() *StatusProvider {
	return &StatusProvider{}
}
