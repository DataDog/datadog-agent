// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package status contains the gNMI status provider.
package status

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

type provider struct {
	registry *Registry
}

// NewProvider returns a status provider rendering devices from the registry.
func NewProvider(registry *Registry) status.Provider {
	return provider{registry: registry}
}

// Name returns the name.
func (provider) Name() string {
	return "GNMI"
}

// Section returns the section.
func (provider) Section() string {
	return "GNMI"
}

func (p provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (p provider) populateStatus(stats map[string]interface{}) {
	stats["devices"] = p.registry.listDevicesForDisplay()
}

// JSON populates the status map.
func (p provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

// Text renders the text output.
func (p provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "gnmi.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output.
func (p provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "gnmiHTML.tmpl", buffer, p.getStatusInfo())
}
