// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscovery fetch information needed to render the 'autodiscovery' section of the status page.
package autodiscovery

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	stats["adEnabledFeatures"] = config.GetDetectedFeatures()
	if common.AC != nil {
		stats["adConfigErrors"] = common.AC.GetAutodiscoveryErrors()
	}
	stats["filterErrors"] = containers.GetFilterErrors()
}

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// GetProvider if agent is running in a container environment returns status.Provider otherwise returns NoopProvider
func GetProvider() status.Provider {
	if config.IsContainerized() {
		return Provider{}
	}

	return status.NoopProvider{}
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	PopulateStatus(stats)

	return stats
}

// Name returns the name
func (p Provider) Name() string {
	return "Autodiscovery"
}

// Section return the section
func (p Provider) Section() string {
	return "Autodiscovery"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	PopulateStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "autodiscovery.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return nil
}
