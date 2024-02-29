// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscovery fetch information needed to render the 'autodiscovery' section of the status page.
package autodiscovery

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

//go:embed status_templates
var templatesFS embed.FS

// StatusProvider provides the functionality to populate the status output
type StatusProvider struct {
	ac Component
}

func (p StatusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (p StatusProvider) populateStatus(stats map[string]interface{}) {
	stats["adEnabledFeatures"] = config.GetDetectedFeatures()
	if p.ac.IsStarted() {
		stats["adConfigErrors"] = p.ac.GetAutodiscoveryErrors()
	}
	stats["filterErrors"] = containers.GetFilterErrors()
}

// Name returns the name
func (p StatusProvider) Name() string {
	return "Autodiscovery"
}

// Section return the section
func (p StatusProvider) Section() string {
	return "Autodiscovery"
}

// JSON populates the status map
func (p StatusProvider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

// Text renders the text output
func (p StatusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "autodiscovery.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p StatusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
