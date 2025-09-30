// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fleetstatusimpl implements the fleetstatus component interface
package fleetstatusimpl

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	daemonchecker "github.com/DataDog/datadog-agent/comp/updater/daemonchecker/def"
)

// Requires defines the dependencies for the fleetstatus component
type Requires struct {
	Config        config.Component
	DaemonChecker daemonchecker.Component
}

// Provides defines the output of the fleetstatus component
type Provides struct {
	Status status.InformationProvider
}

type statusProvider struct {
	Config        config.Component
	DaemonChecker daemonchecker.Component
}

// NewComponent creates a new fleetstatus component
func NewComponent(reqs Requires) Provides {
	sp := &statusProvider{
		Config:        reqs.Config,
		DaemonChecker: reqs.DaemonChecker,
	}

	return Provides{
		Status: status.NewInformationProvider(sp),
	}
}

//go:embed status_templates
var templatesFS embed.FS

func (sp statusProvider) getStatusInfo(html bool) map[string]interface{} {
	stats := make(map[string]interface{})

	sp.populateStatus(stats)
	stats["HTML"] = html

	return stats
}

// Name returns the name
func (sp statusProvider) Name() string {
	return "Fleet Automation"
}

// Section return the section
func (sp statusProvider) Section() string {
	return "Fleet Automation"
}

// JSON populates the status map
func (sp statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	sp.populateStatus(stats)

	return nil
}

// Text renders the text output
func (sp statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "fleetstatus.tmpl", buffer, sp.getStatusInfo(false))
}

// HTML renders the html output
func (sp statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "fleetstatus.tmpl", buffer, sp.getStatusInfo(true))
}

func (sp statusProvider) populateStatus(stats map[string]interface{}) {
	status := make(map[string]interface{})

	remoteManagementEnabled := isRemoteManagementEnabled(sp.Config)
	isInstallerRunning := false
	isInstallerRunning, _ = sp.DaemonChecker.IsRunning()

	status["remoteManagementEnabled"] = remoteManagementEnabled
	status["installerRunning"] = isInstallerRunning
	status["fleetAutomationEnabled"] = remoteManagementEnabled && isInstallerRunning
	stats["fleetAutomationStatus"] = status
}

func isRemoteManagementEnabled(conf config.Component) bool {
	return conf.GetBool("remote_updates")
}
