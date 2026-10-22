// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// statusProvider exposes the configuration warnings in the agent status.
type statusProvider struct {
	config Reader
}

// Name returns the name of the status provider
func (s statusProvider) Name() string {
	return "Configuration"
}

// Section returns the section of the status provider
func (s statusProvider) Section() string {
	return "configuration"
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	stats["configFile"] = s.config.ConfigFileUsed()
	stats["extraConfigFiles"] = s.config.ExtraConfigFilesUsed()
	stats["configWarnings"] = s.config.Warnings()
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	s.populateStatus(stats)
	return stats
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "config.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the HTML output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "configHTML.tmpl", buffer, s.getStatusInfo())
}
