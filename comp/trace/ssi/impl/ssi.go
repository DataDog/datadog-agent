// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ssiimpl implements the ssi component interface
package ssiimpl

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/status"
	defssi "github.com/DataDog/datadog-agent/comp/trace/ssi/def"
)

// Requires defines the dependencies for the ssi component
type Requires struct {
	Config config.Component
}

// Provides defines the output of the ssi component
type Provides struct {
	Component defssi.Component

	StatusProvider status.InformationProvider
	FlareProvider  flaretypes.Provider
}

type ssiComp struct {
	config config.Component
}

// NewComponent creates a new ssi component
func NewComponent(reqs Requires) (Provides, error) {
	comp := ssiComp{
		config: reqs.Config,
	}

	return Provides{
		Component:      comp,
		StatusProvider: status.NewInformationProvider(comp),
		FlareProvider:  flaretypes.NewProvider(comp.fillFlare),
	}, nil
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (ssi ssiComp) Name() string {
	return "Single Step Instrumentation" // TODO: fix if needed
}

// Section return the section
func (ssi ssiComp) Section() string {
	return "Single Step Instrumentation" // TODO: fix if needed
}

// JSON populates the status map
func (ssi ssiComp) JSON(_ bool, stats map[string]interface{}) error {
	ssi.populateStatus(stats)
	return nil
}

// Text renders the text output
func (ssi ssiComp) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "ssi.tmpl", buffer, ssi.getStatusInfo())
}

// HTML renders the html output
func (ssi ssiComp) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "ssiHTML.tmpl", buffer, ssi.getStatusInfo())
}

func (ssi ssiComp) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	ssi.populateStatus(stats)

	return stats
}

func (ssi ssiComp) populateStatus(stats map[string]interface{}) {
	// TODO: populate statusData
	status := make(map[string]interface{})
	status["ssiStatus"] = "disabled"
	if isSSIEnabled(ssi.config) {
		status["ssiStatus"] = "enabled"
		status["ociInstallerVersion"] = "" //TODO: fetch and assign the version here
		status["injectorVersion"] = ""     //TODO: fetch and assign the version here
		if installedLibraries, ok := fetchInstalledLibraries(ssi.config); ok {
			status["libraries"] = installedLibraries
		}
	}

	stats["singleStepInstrumentation"] = status
}

func isSSIEnabled(_ config.Component) bool {
	// TODO: implement
	return false
}

func fetchInstalledLibraries(_ config.Component) (map[string]string, bool) {
	//TODO: implement
	return nil, false
}

// fillFlare add the Configuration files to flares.
func (ssi ssiComp) fillFlare(fb flaretypes.FlareBuilder) error {
	if !isSSIEnabled(ssi.config) {
		fb.AddFile("ssi.status", []byte("SSI is not enabled"))
	}
	//TODO: fill flare with SSI related information
	return nil
}
