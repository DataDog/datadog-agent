// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package status contains the gNMI status provider.
package status

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output.
type Provider struct{}

// GetProvider returns a status provider when gNMI is enabled, otherwise nil.
func GetProvider(conf config.Component) status.Provider {
	if !conf.GetBool("network_devices.gnmi.enabled") {
		return nil
	}

	return Provider{}
}

// Name returns the name.
func (Provider) Name() string {
	return "GNMI"
}

// Section returns the section.
func (Provider) Section() string {
	return "GNMI"
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (Provider) populateStatus(stats map[string]interface{}) {
	stats["devices"] = listDevicesForDisplay()
}

// JSON populates the status map.
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

// Text renders the text output.
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "gnmi.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output.
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "gnmiHTML.tmpl", buffer, p.getStatusInfo())
}
