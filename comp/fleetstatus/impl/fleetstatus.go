// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fleetstatusimpl implements the fleetstatus component interface
package fleetstatusimpl

import (
	"embed"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcstatus"
	installerexec "github.com/DataDog/datadog-agent/comp/updater/installerexec/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the fleetstatus component
type Requires struct {
	Config config.Component

	InstallerExec option.Option[installerexec.Component]
	RemoteConfig  rcstatus.Component
}

// Provides defines the output of the fleetstatus component
type Provides struct {
	Status status.InformationProvider
}

type statusProvider struct {
	Config        config.Component
	InstallerExec installerexec.Component
}

// NewComponent creates a new fleetstatus component
func NewComponent(reqs Requires) Provides {
	 installerExec, _ := reqs.InstallerExec.Get()
	sp := &statusProvider{
		Config:        reqs.Config,
		InstallerExec: installerExec,
	}

	return Provides{
		Status: status.NewInformationProvider(sp),
	}
}

//go:embed status_templates
var templatesFS embed.FS

func (sp statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	sp.populateStatus(stats)

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
	return status.RenderText(templatesFS, "fleetstatus.tmpl", buffer, sp.getStatusInfo())
}

// HTML renders the html output
func (sp statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "fleetstatusHTML.tmpl", buffer, sp.getStatusInfo())
}

func (sp statusProvider) populateStatus(stats map[string]interface{}) {
	status := make(map[string]interface{})

	remoteManagementEnabled := isRemoteManagementEnabled(sp.Config)
	remoteConfigEnabled := isRemoteConfigEnabled()
	installerRunning := isInstallerRunning(sp.InstallerExec)

	status["remoteManagementEnabled"] = remoteManagementEnabled
	status["remoteConfigEnabled"] = remoteConfigEnabled
	status["installerRunning"] = installerRunning

	status["fleetAutomationEnabled"] = remoteManagementEnabled && remoteConfigEnabled && installerRunning

	stats["fleetAutomationStatus"] = status
}

func isRemoteManagementEnabled(conf config.Component) bool {
	return conf.GetBool("remote_updates")
}

func isRemoteConfigEnabled() bool {
	return expvar.Get("remoteConfigStatus") != nil
}

func isInstallerRunning(installer installerexec.Component) bool {
	return installer != nil
}
