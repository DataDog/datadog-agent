// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

// RemoteWindowsInstallerAssertions provides assertions for the Datadog Installer on Windows.
type RemoteWindowsInstallerAssertions struct {
	*RemoteWindowsBinaryAssertions
}

func (d *RemoteWindowsInstallerAssertions) execute(cmd string, options ...client.ExecuteOption) (string, error) {
	output, err := d.remoteHost.Execute(fmt.Sprintf("& \"%s\" %s", d.binaryPath, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// Status provides assertions on the status output of the Datadog Installer.
func (d *RemoteWindowsInstallerAssertions) Status() *RemoteWindowsInstallerStatusAssertions {
	output, err := d.execute("status --json")
	d.require.NoError(err)
	status, err := parseStatusOutput(output)
	if err != nil {
		d.context.T().Logf("Failed to parse status output: %s", output)
		d.require.NoError(err)
	}
	return &RemoteWindowsInstallerStatusAssertions{
		RemoteWindowsInstallerAssertions: d,
		status:                           status,
	}
}

// RemoteWindowsInstallerStatusAssertions provides assertions on the status output of the Datadog Installer.
type RemoteWindowsInstallerStatusAssertions struct {
	*RemoteWindowsInstallerAssertions
	status installerStatus
}

// HasPackage verifies that a package is present in the status output.
func (d *RemoteWindowsInstallerStatusAssertions) HasPackage(name string) *RemoteWindowsInstallerPackageAssertions {
	d.context.T().Helper()
	d.require.Contains(d.status.Packages.States, name)
	return &RemoteWindowsInstallerPackageAssertions{
		RemoteWindowsInstallerStatusAssertions: d,
		name:                                   name,
	}
}

// RemoteWindowsInstallerPackageAssertions provides assertions on a package in the status output of the Datadog Installer.
type RemoteWindowsInstallerPackageAssertions struct {
	*RemoteWindowsInstallerStatusAssertions
	name string
}

// WithStableVersionEqual verifies the stable version of a package matches what's expected.
func (d *RemoteWindowsInstallerPackageAssertions) WithStableVersionEqual(version string) *RemoteWindowsInstallerPackageAssertions {
	d.context.T().Helper()
	d.require.Equal(version, d.status.Packages.States[d.name].Stable.Version, "expected matching stable version for package %s", d.name)
	return d
}

// WithExperimentVersionEqual verifies the experiment version of a package matches what's expected.
func (d *RemoteWindowsInstallerPackageAssertions) WithExperimentVersionEqual(version string) *RemoteWindowsInstallerPackageAssertions {
	d.context.T().Helper()
	d.require.Equal(version, d.status.Packages.States[d.name].Experiment.Version, "expected matching experiment version for package %s", d.name)
	return d
}

// WithStableVersionMatchPredicate verifies the stable version of a package by using a predicate function.
func (d *RemoteWindowsInstallerPackageAssertions) WithStableVersionMatchPredicate(predicate func(version string)) *RemoteWindowsInstallerPackageAssertions {
	d.context.T().Helper()
	predicate(d.status.Packages.States[d.name].Stable.Version)
	return d
}

// WithExperimentVersionMatchPredicate verifies the experiment version of a package by using a predicate function.
func (d *RemoteWindowsInstallerPackageAssertions) WithExperimentVersionMatchPredicate(predicate func(version string)) *RemoteWindowsInstallerPackageAssertions {
	d.context.T().Helper()
	predicate(d.status.Packages.States[d.name].Experiment.Version)
	return d
}

// HasConfigState asserts that a package config is present in the status output.
func (d *RemoteWindowsInstallerStatusAssertions) HasConfigState(name string) *RemoteWindowsInstallerConfigStateAssertions {
	d.context.T().Helper()
	d.require.Contains(d.status.Packages.ConfigStates, name)
	return &RemoteWindowsInstallerConfigStateAssertions{
		RemoteWindowsInstallerStatusAssertions: d,
		name:                                   name,
	}
}

// RemoteWindowsInstallerConfigStateAssertions provides assertions on a package's config state in the status output.
type RemoteWindowsInstallerConfigStateAssertions struct {
	*RemoteWindowsInstallerStatusAssertions
	name string
}

// WithStableConfigEqual asserts the stable config of a package matches what's expected.
func (d *RemoteWindowsInstallerConfigStateAssertions) WithStableConfigEqual(config string) *RemoteWindowsInstallerConfigStateAssertions {
	d.context.T().Helper()
	d.require.Equal(config, d.status.Packages.ConfigStates[d.name].Stable, "expected matching stable config for package %s", d.name)
	return d
}

// WithExperimentConfigEqual asserts the experiment config of a package matches
func (d *RemoteWindowsInstallerConfigStateAssertions) WithExperimentConfigEqual(config string) *RemoteWindowsInstallerConfigStateAssertions {
	d.context.T().Helper()
	d.require.Equal(config, d.status.Packages.ConfigStates[d.name].Experiment, "expected matching experiment config for package %s", d.name)
	return d
}

type packageStatus struct {
	States       map[string]packageVersionStatus   `json:"package_states"`
	ConfigStates map[string]stableExperimentStatus `json:"config_states"`
}

type stableExperimentStatus struct {
	Stable     string `json:"Stable"`
	Experiment string `json:"Experiment"`
}

type packageVersionStatus struct {
	Stable     versionState `json:"stable"`
	Experiment versionState `json:"experiment"`
}

type versionState struct {
	Version    string   `json:"version"`
	Extensions []string `json:"extensions,omitempty"`
}

type installerStatus struct {
	Version  string        `json:"version"`
	Packages packageStatus `json:"packages"`
}

// packageStatusLegacy matches the status --json schema produced by installer
// versions predating the package_states/extensions rework, where each
// package's stable/experiment state was a plain version string.
type packageStatusLegacy struct {
	States       map[string]stateLegacy `json:"states"`
	ConfigStates map[string]stateLegacy `json:"config_states"`
}

type stateLegacy struct {
	Stable     string `json:"stable"`
	Experiment string `json:"experiment"`
}

type installerStatusLegacy struct {
	Version  string              `json:"version"`
	Packages packageStatusLegacy `json:"packages"`
}

// parseStatusOutput parses the json status output of the Datadog Installer.
//
// Mixed-version e2e scenarios can query an installer binary released before
// this schema changed from "states" (plain version strings) to
// "package_states" (nested stable/experiment objects). Unmarshaling old JSON
// into the new schema doesn't error - the missing key just leaves States nil
// - so detect that case and fall back to the legacy schema.
func parseStatusOutput(output string) (installerStatus, error) {
	var status installerStatus

	err := json.Unmarshal([]byte(output), &status)
	if err != nil {
		return status, err
	}
	if len(status.Packages.States) == 0 {
		var legacy installerStatusLegacy
		if err := json.Unmarshal([]byte(output), &legacy); err == nil && len(legacy.Packages.States) > 0 {
			status = fromLegacyStatus(legacy)
		}
	}
	return status, nil
}

func fromLegacyStatus(legacy installerStatusLegacy) installerStatus {
	states := make(map[string]packageVersionStatus, len(legacy.Packages.States))
	for name, s := range legacy.Packages.States {
		states[name] = packageVersionStatus{
			Stable:     versionState{Version: s.Stable},
			Experiment: versionState{Version: s.Experiment},
		}
	}
	configStates := make(map[string]stableExperimentStatus, len(legacy.Packages.ConfigStates))
	for name, s := range legacy.Packages.ConfigStates {
		configStates[name] = stableExperimentStatus(s)
	}
	return installerStatus{
		Version: legacy.Version,
		Packages: packageStatus{
			States:       states,
			ConfigStates: configStates,
		},
	}
}
