// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jmx

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "JMX"
}

// Section return the section
func (Provider) Section() string {
	return "JMX Fetch"
}

func (p Provider) getStatusInfo(verbose bool) map[string]interface{} {
	stats := make(map[string]interface{})

	PopulateStatus(stats)

	stats["verbose"] = verbose

	return stats
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	PopulateStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(verbose bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "jmx.tmpl", buffer, p.getStatusInfo(verbose))
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "jmxHTML.tmpl", buffer, p.getStatusInfo(false))
}
