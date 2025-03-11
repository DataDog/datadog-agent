// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const (
	packageAPMInject = "datadog-apm-inject"
)

var (
	loaderRelativePath = []string{"injldr.exe"}
)

func getInjectTargetPath(target string) string {
	return filepath.Join(paths.PackagesPath, packageAPMInject, target)
}

func getInjectExecutablePath(installDir string) string {
	return filepath.Join(append([]string{installDir}, loaderRelativePath...)...)
}

// SetupAPMInjector noop
func SetupAPMInjector(_ context.Context) error {
	return nil
}

// RemoveAPMInjector noop
func RemoveAPMInjector(_ context.Context) error {
	return nil
}

// InstrumentAPMInjector instruments the APM injector
func InstrumentAPMInjector(ctx context.Context, _ string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_injector")
	defer func() { span.Finish(err) }()
	var installDir string
	installDir, err = filepath.EvalSymlinks(getInjectTargetPath("stable"))
	if err != nil {
		return err
	}
	execInject := exec.NewApmInjectExec(getInjectExecutablePath(installDir))
	_, err = execInject.Instrument(ctx)
	if err != nil {
		return err
	}
	return nil
}

// UninstrumentAPMInjector uninstruments the APM injector
func UninstrumentAPMInjector(ctx context.Context, _ string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_injector")
	defer func() { span.Finish(err) }()
	var installDir string
	installDir, err = filepath.EvalSymlinks(getInjectTargetPath("stable"))
	if err != nil {
		return err
	}
	execInject := exec.NewApmInjectExec(getInjectExecutablePath(installDir))
	_, err = execInject.Uninstrument(ctx)
	if err != nil {
		return err
	}
	return nil
}
