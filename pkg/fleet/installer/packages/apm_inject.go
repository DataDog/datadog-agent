// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/apminject"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// SetupAPMInjector sets up the injector at bootstrap
func SetupAPMInjector(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "setup_injector")
	defer func() { span.Finish(err) }()
	installer := apminject.NewInstaller()
	defer func() { installer.Finish(err) }()
	return installer.Setup(ctx)
}

// RemoveAPMInjector removes the APM injector
func RemoveAPMInjector(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "remove_injector")
	defer func() { span.Finish(err) }()
	installer := apminject.NewInstaller()
	defer func() { installer.Finish(err) }()
	return installer.Remove(ctx)
}

// InstrumentAPMInjector instruments the APM injector
func InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_injector")
	defer func() { span.Finish(err) }()
	installer := apminject.NewInstaller()
	installer.Env.InstallScript.APMInstrumentationEnabled = method
	defer func() { installer.Finish(err) }()
	return installer.Instrument(ctx)
}

// UninstrumentAPMInjector uninstruments the APM injector
func UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_injector")
	defer func() { span.Finish(err) }()
	installer := apminject.NewInstaller()
	installer.Env.InstallScript.APMInstrumentationEnabled = method
	defer func() { installer.Finish(err) }()
	return installer.Uninstrument(ctx)
}
