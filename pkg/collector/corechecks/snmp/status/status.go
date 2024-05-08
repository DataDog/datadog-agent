// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package status contains the SNMP Profiles status provider
package status

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "SNMP Profiles"
}

// Section return the section
func (Provider) Section() string {
	return "SNMP Profiles"
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (Provider) populateStatus(stats map[string]interface{}) {
	profiles := make(map[string]string)

	snmpProfileErrorsVar := expvar.Get("snmpProfileErrors")
	if snmpProfileErrorsVar != nil {
		snmpProfileErrorsJSON := []byte(snmpProfileErrorsVar.String())
		json.Unmarshal(snmpProfileErrorsJSON, &profiles) //nolint:errcheck
		stats["snmpProfiles"] = profiles
	}
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "profiles.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "profilesHTML.tmpl", buffer, p.getStatusInfo())
}
