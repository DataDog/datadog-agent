// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	pkgExec "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	apmInjectPackage = hooks{
		preInstall:          preInstallAPMInject,
		postInstall:         postInstallAPMInject,
		preRemove:           preRemoveAPMInject,
		postStartExperiment: postStartExperimentAPMInject,
		preStopExperiment:   preStopExperimentAPMInject,
		postStopExperiment:  postStopExperimentAPMInject,
	}
)

const (
	packageAPMInject = "datadog-apm-inject"
	installerExe     = "ddinjector-installer.exe"
)

func getAPMInjectExecutablePath(installDir string) string {
	return filepath.Join(installDir, installerExe)
}

func getAPMInjectTargetPath(target string) string {
	return filepath.Join(paths.PackagesPath, packageAPMInject, target)
}

// preInstallAPMInject is called before the APM inject package is installed
func preInstallAPMInject(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("pre_install_apm_inject")
	defer func() { span.Finish(err) }()
	// uninstrument the injector if it is already installed
	// ignore error as it might not be installed
	_ = UninstrumentAPMInjector(ctx.Context, env.APMInstrumentationEnabledIIS)
	return nil
}

// postInstallAPMInject is called after the APM inject package is installed
func postInstallAPMInject(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("setup_apm_inject")
	defer func() { span.Finish(err) }()

	// Get the installer path
	packagePath, err := filepath.EvalSymlinks(getAPMInjectTargetPath("stable"))
	if err != nil {
		return err
	}

	// Run the installer to install the driver
	injectorExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(packagePath))
	injectorExec.WithDDInjectorPackage(packagePath)
	injectorExec.WithDDAgentVersion(version.AgentPackageVersion)
	_, err = injectorExec.Install(ctx.Context)
	if err != nil {
		return fmt.Errorf("failed to install APM inject driver: %w", err)
	}

	return nil
}

// preRemoveAPMInject is called before the APM inject package is removed
func preRemoveAPMInject(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("remove_apm_inject")
	defer func() { span.Finish(err) }()

	// Get the installer path
	packagePath, err := filepath.EvalSymlinks(getAPMInjectTargetPath("stable"))
	if err != nil {

		// If the remove is being retried after a failed first attempt, the stable symlink may have been removed
		// so we do not consider this an error
		if errors.Is(err, fs.ErrNotExist) {
			log.Warn("Stable symlink does not exist, assuming the package has already been partially removed and skipping UninstallProduct")
			return nil
		}
		return err
	}

	// Run the installer to uninstall the driver
	injectorExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(packagePath))
	_, err = injectorExec.Uninstall(ctx.Context)
	if err != nil {
		return fmt.Errorf("failed to uninstall APM inject driver: %w", err)
	}

	return nil
}

// postStartExperimentAPMInject starts an APM inject experiment by installing the experiment version
func postStartExperimentAPMInject(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("start_apm_inject_experiment")
	defer func() { span.Finish(err) }()

	// Get the stable package path and remove it before installing the experiment version
	packageStablePath, err := filepath.EvalSymlinks(getAPMInjectTargetPath("stable"))
	if err != nil {
		return err
	}

	// Run the installer to uninstall the stable driver
	injectorStableExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(packageStablePath))
	_, err = injectorStableExec.Uninstall(ctx.Context)
	if err != nil {
		return fmt.Errorf("failed to uninstall stable APM inject driver: %w", err)
	}

	// Get the experiment package path
	packagePath, err := filepath.EvalSymlinks(getAPMInjectTargetPath("experiment"))
	if err != nil {
		return err
	}

	// Run the installer to install the experiment driver
	injectorExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(packagePath))
	injectorExec.WithDDInjectorPackage(packagePath)
	injectorExec.WithDDAgentVersion(version.AgentPackageVersion)
	_, err = injectorExec.Install(ctx.Context)
	if err != nil {
		return fmt.Errorf("failed to install APM inject driver experiment: %w", err)
	}

	return nil
}

// preStopExperimentAPMInject stops an APM inject experiment by reinstalling the stable version
func preStopExperimentAPMInject(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("stop_apm_inject_experiment")
	defer func() { span.Finish(err) }()

	// Uninstall the experiment driver
	experimentInjectorExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(getAPMInjectTargetPath("experiment")))
	_, err = experimentInjectorExec.Uninstall(ctx.Context)
	if err != nil {
		return fmt.Errorf("failed to uninstall APM inject driver experiment: %w", err)
	}

	// Get the stable package path
	packagePath, err := filepath.EvalSymlinks(getAPMInjectTargetPath("stable"))
	if err != nil {
		return err
	}

	// Run the installer to reinstall the stable driver
	injectorExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(packagePath))
	injectorExec.WithDDInjectorPackage(packagePath)
	injectorExec.WithDDAgentVersion(version.AgentPackageVersion)
	_, err = injectorExec.Install(ctx.Context)
	if err != nil {
		return fmt.Errorf("failed to reinstall stable APM inject driver: %w", err)
	}

	return nil
}

// postStopExperimentAPMInject is called after the APM inject experiment is stopped
func postStopExperimentAPMInject(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("stop_apm_inject_experiment")
	defer func() { span.Finish(err) }()

	// we don't have anything to do here
	return nil
}

func instrumentAPMInject(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_apm_inject")
	defer func() { span.Finish(err) }()

	// Get the stable package path
	packagePath, err := filepath.EvalSymlinks(getAPMInjectTargetPath("stable"))
	if err != nil {
		return err
	}

	// Run the installer to install the stable driver
	injectorExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(packagePath))
	injectorExec.WithDDInjectorPackage(packagePath)
	injectorExec.WithDDAgentVersion(version.AgentPackageVersion)
	_, err = injectorExec.Install(ctx)
	if err != nil {
		return fmt.Errorf("failed to install stable APM inject driver: %w", err)
	}
	return nil
}

func uninstrumentAPMInject(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_apm_inject")
	defer func() { span.Finish(err) }()

	// Get the stable package path
	packagePath, err := filepath.EvalSymlinks(getAPMInjectTargetPath("stable"))
	if err != nil {
		return err
	}

	// Run the installer to install the stable driver
	injectorExec := pkgExec.NewAPMInjectExec(getAPMInjectExecutablePath(packagePath))
	injectorExec.WithDDInjectorPackage(packagePath)
	injectorExec.WithDDAgentVersion(version.AgentPackageVersion)
	_, err = injectorExec.Uninstall(ctx)
	if err != nil {
		return fmt.Errorf("failed to uninstall stable APM inject driver: %w", err)
	}
	return nil
}

// InstrumentAPMInjector instruments the APM injector for IIS on Windows
func InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_injector")
	defer func() { span.Finish(err) }()

	switch method {
	case env.APMInstrumentationEnabledIIS:
		err = instrumentDotnetLibrary(ctx, "stable")
		if err != nil {
			return err
		}
	case env.APMInstrumentationEnabledHost:
		err = instrumentAPMInject(ctx)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unsupported method: %s", method)

	}

	return nil
}

// UninstrumentAPMInjector un-instruments the APM injector for IIS on Windows
func UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_injector")
	defer func() { span.Finish(err) }()

	switch method {
	case env.APMInstrumentationEnabledIIS:
		err = uninstrumentDotnetLibrary(ctx, "stable")
		if err != nil {
			return err
		}
	case env.APMInstrumentationEnabledHost:
		err = uninstrumentAPMInject(ctx)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unsupported method: %s", method)

	}
	return nil
}
