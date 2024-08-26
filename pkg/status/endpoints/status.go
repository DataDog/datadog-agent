// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package endpoints fetch information needed to render the 'endpoints' section of the status page.
package endpoints

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	endpoints, err := utils.GetMultipleEndpoints(config.Datadog())
	if err != nil {
		stats["endpointsInfos"] = nil
		return
	}

	endpointsInfos := make(map[string]interface{})

	// obfuscate the api keys
	for endpoint, keys := range endpoints {
		for i, key := range keys {
			if len(key) > 5 {
				keys[i] = key[len(key)-5:]
			}
		}
		endpointsInfos[endpoint] = keys
	}
	stats["endpointsInfos"] = endpointsInfos
}

// Provider provides the functionality to populate the status output
type Provider struct{}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (Provider) Name() string {
	return "Endpoints"
}

// Section return the section
func (Provider) Section() string {
	return "endpoints"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	PopulateStatus(stats)

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "endpoints.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "endpointsHTML.tmpl", buffer, getStatusInfo())
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	PopulateStatus(stats)

	return stats
}
