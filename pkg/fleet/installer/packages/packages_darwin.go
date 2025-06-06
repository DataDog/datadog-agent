// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

var (
	// packagesHooks is a map of package names to their hooks
	packagesHooks = map[string]hooks{}

	// AsyncPreRemoveHooks is called before a package is removed from the disk.
	// It can block the removal of the package files until a condition is met without blocking
	// the rest of the uninstall or upgrade process.
	// Today this is only useful for the dotnet tracer on windows and generally *SHOULD BE AVOIDED*.
	AsyncPreRemoveHooks = map[string]repository.PreRemoveHook{}

	// packageCommands is a map of package names to their command handlers
	packageCommands = map[string]PackageCommandHandler{}
)

// InstrumentAPMInjector instruments the APM injector
func InstrumentAPMInjector(_ context.Context, _ string) (err error) {
	return nil
}

// UninstrumentAPMInjector uninstruments the APM injector
func UninstrumentAPMInjector(_ context.Context, _ string) (err error) {
	return nil
}
