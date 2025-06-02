// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides the installer types and interfaces. This is to avoid importing too many dependencies
// when importing these types in other packages.
package types

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

// Installer is a package manager that installs and uninstalls packages.
type Installer interface {
	IsInstalled(ctx context.Context, pkg string) (bool, error)
	Status(ctx context.Context, debug bool) (*InstallerStatus, error)

	AvailableDiskSpace() (uint64, error)
	State(ctx context.Context, pkg string) (repository.State, error)
	States(ctx context.Context) (map[string]repository.State, error)
	ConfigState(ctx context.Context, pkg string) (repository.State, error)
	ConfigStates(ctx context.Context) (map[string]repository.State, error)

	Install(ctx context.Context, url string, args []string) error
	ForceInstall(ctx context.Context, url string, args []string) error
	SetupInstaller(ctx context.Context, path string) error
	Remove(ctx context.Context, pkg string) error
	Purge(ctx context.Context)

	InstallExperiment(ctx context.Context, url string) error
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	InstallConfigExperiment(ctx context.Context, pkg string, version string, rawConfig []byte) error
	RemoveConfigExperiment(ctx context.Context, pkg string) error
	PromoteConfigExperiment(ctx context.Context, pkg string) error

	GarbageCollect(ctx context.Context) error

	InstrumentAPMInjector(ctx context.Context, method string) error
	UninstrumentAPMInjector(ctx context.Context, method string) error

	Close() error
}
