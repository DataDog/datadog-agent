// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var apmLibraryDotnetPackage = hooks{
	postInstall:         postInstallAPMLibraryDotnet,
	preRemove:           preRemoveAPMLibraryDotnet,
	postStartExperiment: postStartExperimentAPMLibraryDotnet,
	preStopExperiment:   preStopExperimentAPMLibraryDotnet,
}

const (
	packageAPMLibraryDotnet = "datadog-apm-library-dotnet"
)

var (
	installerRelativePath = []string{"installer", "Datadog.FleetInstaller.exe"}
)

func getTargetPath(target string) string {
	return filepath.Join(paths.PackagesPath, packageAPMLibraryDotnet, target)
}

func getExecutablePath(installDir string) string {
	return filepath.Join(append([]string{installDir}, installerRelativePath...)...)
}

func getLibraryPath(installDir string) string {
	return filepath.Join(installDir, "library")
}

// postInstallAPMLibraryDotnet runs on the first install of the .NET APM library after the files are laid out on disk.
func postInstallAPMLibraryDotnet(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("setup_apm_library_dotnet")
	defer func() { span.Finish(err) }()
	// Register GAC + set env variables
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("stable"))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	_, err = dotnetExec.InstallVersion(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	return instrumentDotnetLibraryIfNeeded(ctx, "stable", ctx.Upgrade)
}

// postStartExperimentAPMLibraryDotnet starts a .NET APM library experiment.
func postStartExperimentAPMLibraryDotnet(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("start_apm_library_dotnet_experiment")
	defer func() { span.Finish(err) }()
	// Register GAC + set env variables new version
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("experiment"))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	_, err = dotnetExec.InstallVersion(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	return instrumentDotnetLibraryIfNeeded(ctx, "experiment", true)
}

// preStopExperimentAPMLibraryDotnet stops a .NET APM library experiment.
func preStopExperimentAPMLibraryDotnet(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("stop_apm_library_dotnet_experiment")
	defer func() { span.Finish(err) }()
	// Re-register GAC + set env variables of stable version
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("stable"))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	_, err = dotnetExec.InstallVersion(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	return instrumentDotnetLibraryIfNeeded(ctx, "stable", true)
}

// preRemoveAPMLibraryDotnet uninstalls the .NET APM library
// This function only disable injection, the cleanup for each version is done by the PreRemoveHook
func preRemoveAPMLibraryDotnet(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("remove_apm_library_dotnet")
	defer func() { span.Finish(err) }()
	_, err = filepath.EvalSymlinks(getTargetPath("stable"))
	if err != nil {
		// If the remove is being retried after a failed first attempt, the stable symlink may have been removed
		// so we do not consider this an error
		if errors.Is(err, fs.ErrNotExist) {
			log.Warn("Stable symlink does not exist, assuming the package has already been partially removed and skipping UninstallProduct")
			return nil
		}
		return err
	}
	// TODO always uninstrument IIS just in cas
	return uninstrumentDotnetLibraryIfNeeded(ctx.Context, "stable")
}

// asyncPreRemoveHookAPMLibraryDotnet runs before the garbage collector deletes the package files for a version.
// It checks that it's safe to delete it and cleans up the external dependencies of the package.
func asyncPreRemoveHookAPMLibraryDotnet(ctx context.Context, pkgRepositoryPath string) (bool, error) {
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(pkgRepositoryPath))
	exitCode, err := dotnetExec.UninstallVersion(ctx, getLibraryPath(pkgRepositoryPath))
	if err != nil {
		// We only block deletion if we could not delete the native loader files
		// cf https://github.com/DataDog/dd-trace-dotnet/blob/master/tracer/src/Datadog.FleetInstaller/ReturnCode.cs#L14
		const errorRemovingNativeLoaderFiles = 2
		shouldDelete := exitCode != errorRemovingNativeLoaderFiles
		return shouldDelete, err
	}
	return true, nil
}

func instrumentDotnetLibraryIfNeeded(ctx context.Context, target string, upgrade bool) (err error) {
	envInst := env.FromEnv()
	method := envInst.InstallScript.APMInstrumentationEnabled
	return updateInstrumentation(ctx, method, target, upgrade)
}

func updateInstrumentation(ctx context.Context, newMethod, target string, upgrade bool) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "update_instrumentation")
	defer func() { span.Finish(err) }()

	// TODO What if it's a reinstall and the instrumentation method config was not properly cleaned up by the previous installation?
	// Check if a an instrumentation method was set during a previous installation
	var currentMethod string
	currentMethod, err = getAPMInstrumentationMethod()
	if err != nil {
		return fmt.Errorf("could not get current instrumentation method: %w", err)
	}

	// The first version of SSI for IIS did not store the configured instrumentation
	// So if it's an upgrade and not a reinstall, we assume it's IIS instrumentation
	if currentMethod == env.APMInstrumentationNotSet {
		if upgrade {
			currentMethod = env.APMInstrumentationEnabledIIS
		} else {
			currentMethod = env.APMInstrumentationDisabled
		}
	}

	fmt.Printf("currentMethod: %s, newMethod: %s\n", currentMethod, newMethod)

	// If no new method is provided, defaults to the current method
	if newMethod == env.APMInstrumentationNotSet {
		newMethod = currentMethod
	}

	if newMethod == env.APMInstrumentationDisabled {
		return setAPMInstrumentationMethod(env.APMInstrumentationDisabled)
	}

	if currentMethod != env.APMInstrumentationDisabled && currentMethod != newMethod {
		err = uninstrumentDotnetLibrary(ctx, currentMethod, target)
		// TODO show an error but do not fail the installation
		if err != nil {
			log.Errorf("could not uninstrument dotnet library: %w", err)
			span.SetTag("uninstrumentation_error", err.Error())
		}
	}

	if err = instrumentDotnetLibrary(ctx, newMethod, target); err != nil {
		return fmt.Errorf("could not instrument dotnet library: %w", err)
	}

	return setAPMInstrumentationMethod(newMethod)
}

func instrumentDotnetLibrary(ctx context.Context, method, target string) (err error) {
	switch method {
	case env.APMInstrumentationEnabledIIS:
		var installDir string
		installDir, err = filepath.EvalSymlinks(getTargetPath(target))
		if err != nil {
			return err
		}
		dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
		_, err = dotnetExec.EnableIISInstrumentation(ctx, getLibraryPath(installDir))
		return err
	case env.APMInstrumentationEnabledDotnet:
		var installDir string
		installDir, err = filepath.EvalSymlinks(getTargetPath(target))
		if err != nil {
			return err
		}
		dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
		_, err = dotnetExec.EnableGlobalInstrumentation(ctx, getLibraryPath(installDir))
		return err
	default:
		return fmt.Errorf("unsupported instrumentation method: %s", method)
	}
}

func uninstrumentDotnetLibraryIfNeeded(ctx context.Context, target string) (err error) {
	var method string
	method, err = getAPMInstrumentationMethod()
	if err != nil {
		return err
	}
	fmt.Printf("uninstrumenting dotnet library with method: %s\n", method)
	if method == env.APMInstrumentationNotSet {
		return nil
	}
	err = uninstrumentDotnetLibrary(ctx, method, target)
	if err != nil {
		return fmt.Errorf("could not uninstrument dotnet library: %w", err)
	}
	return unsetAPMInstrumentationMethod()
}

func uninstrumentDotnetLibrary(ctx context.Context, method, target string) (err error) {
	switch method {
	case env.APMInstrumentationEnabledIIS:
		var installDir string
		installDir, err = filepath.EvalSymlinks(getTargetPath(target))
		if err != nil {
			return err
		}
		dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
		_, err := dotnetExec.RemoveIISInstrumentation(ctx)
		return err
	case env.APMInstrumentationEnabledDotnet:
		var installDir string
		installDir, err = filepath.EvalSymlinks(getTargetPath(target))
		if err != nil {
			return err
		}
		dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
		_, err = dotnetExec.RemoveGlobalInstrumentation(ctx)
		return err
	default:
		return fmt.Errorf("unsupported instrumentation method: %s", method)
	}
}
