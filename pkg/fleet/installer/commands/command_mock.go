// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

var (
	// MockInstaller is used for testing
	MockInstaller installer.Installer
)

type installerMock struct{}

// NewInstallerMock returns a new installerMock.
func NewInstallerMock() installer.Installer {
	return &installerMock{}
}

func (m *installerMock) IsInstalled(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *installerMock) AvailableDiskSpace() (uint64, error) {
	return 0, nil
}

func (m *installerMock) State(_ context.Context, pkg string) (repository.State, error) {
	if pkg == "datadog-agent" {
		return repository.State{
			Stable:     "7.31.0",
			Experiment: "7.32.0",
		}, nil
	}
	return repository.State{}, nil
}

func (m *installerMock) ConfigState(_ context.Context, pkg string) (repository.State, error) {
	if pkg == "datadog-agent" {
		return repository.State{
			Stable:     "abc-def-hij",
			Experiment: "",
		}, nil
	}
	return repository.State{}, nil
}

func (m *installerMock) ConfigAndPackageStates(_ context.Context) (*repository.PackageStates, error) {
	return &repository.PackageStates{
		ConfigStates: map[string]repository.State{
			"datadog-agent": {
				Stable:     "abc-def-hij",
				Experiment: "",
			},
		},
		States: map[string]repository.State{
			"datadog-agent": {
				Stable:     "7.31.0",
				Experiment: "7.32.0",
			},
		},
	}, nil
}

func (m *installerMock) Install(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *installerMock) ForceInstall(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *installerMock) SetupInstaller(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) Remove(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) Purge(_ context.Context) {}

func (m *installerMock) InstallExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) RemoveExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) PromoteExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) InstallConfigExperiment(_ context.Context, _ string, _ config.Operations, _ map[string]string) error {
	return nil
}

func (m *installerMock) RemoveConfigExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) PromoteConfigExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) GarbageCollect(_ context.Context) error {
	return nil
}

func (m *installerMock) InstrumentAPMInjector(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) UninstrumentAPMInjector(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) InstallExtensions(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *installerMock) RemoveExtensions(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *installerMock) SaveExtensions(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *installerMock) RestoreExtensions(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *installerMock) Close() error {
	return nil
}
