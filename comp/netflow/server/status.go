// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"embed"
	"encoding/json"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "NetFlow"
}

// Section return the section
func (Provider) Section() string {
	return "NetFlow"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	err := p.getStatus(stats)

	if err != nil {
		return err
	}

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	data, err := p.populateStatus()

	if err != nil {
		return err
	}

	return status.RenderText(templatesFS, "netflow.tmpl", buffer, data)
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	data, err := p.populateStatus()

	if err != nil {
		return err
	}

	return status.RenderHTML(templatesFS, "netflowHTML.tmpl", buffer, data)
}

func (p Provider) getStatus(stats map[string]interface{}) error {
	status := GetStatus()

	var statusMap map[string]interface{}
	statusBytes, err := json.Marshal(status)

	if err != nil {
		return err
	}

	err = json.Unmarshal(statusBytes, &statusMap)

	if err != nil {
		return err
	}

	stats["netflowStats"] = statusMap

	return nil
}

func (p Provider) populateStatus() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	err := p.getStatus(stats)

	return stats, err
}
