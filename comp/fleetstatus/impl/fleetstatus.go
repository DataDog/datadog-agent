// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fleetstatusimpl implements the fleetstatus component interface
package fleetstatusimpl

import (
	"embed"
	"io"
	"sort"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	localapiclient "github.com/DataDog/datadog-agent/comp/updater/localapiclient/def"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Requires defines the dependencies for the fleetstatus component
type Requires struct {
	Config             config.Component
	InstallerAPIClient localapiclient.StatusClient
}

// Provides defines the output of the fleetstatus component
type Provides struct {
	Status status.InformationProvider
}

type statusProvider struct {
	Config             config.Component
	InstallerAPIClient localapiclient.StatusClient
}

type fleetAutomationStatus struct {
	RemoteManagementEnabled bool            `json:"remoteManagementEnabled"`
	InstallerRunning        bool            `json:"installerRunning"`
	FleetAutomationEnabled  bool            `json:"fleetAutomationEnabled"`
	InstallerStatus         installerStatus `json:"installerStatus"`
}

type installerStatus struct {
	Reachable bool                 `json:"reachable"`
	Packages  []*pbgo.PackageState `json:"packages"`
	Error     string               `json:"error,omitempty"`
}

// NewComponent creates a new fleetstatus component
func NewComponent(reqs Requires) Provides {
	sp := &statusProvider{
		Config:             reqs.Config,
		InstallerAPIClient: reqs.InstallerAPIClient,
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
	remoteManagementEnabled := isRemoteManagementEnabled(sp.Config)
	installer := installerStatus{Packages: make([]*pbgo.PackageState, 0)}
	response, err := sp.InstallerAPIClient.Status()
	if err != nil {
		installer.Error = err.Error()
	} else {
		installer.Reachable = true
		installer.Packages = append(installer.Packages, response.RemoteConfigState...)
		sort.Slice(installer.Packages, func(i, j int) bool {
			return installer.Packages[i].GetPackage() < installer.Packages[j].GetPackage()
		})
	}

	stats["fleetAutomationStatus"] = fleetAutomationStatus{
		RemoteManagementEnabled: remoteManagementEnabled,
		InstallerRunning:        installer.Reachable,
		FleetAutomationEnabled:  remoteManagementEnabled && installer.Reachable,
		InstallerStatus:         installer,
	}
}

func isRemoteManagementEnabled(conf config.Component) bool {
	return conf.GetBool("remote_updates")
}
