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
	return instrumentDotnetLibraryIfNeeded(ctx, "stable")
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
	return instrumentDotnetLibraryIfNeeded(ctx, "experiment")
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
	return instrumentDotnetLibraryIfNeeded(ctx, "stable")
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

func instrumentDotnetLibraryIfNeeded(ctx context.Context, target string) (err error) {
	// Check if a an injection method was set during a previous installation
	var currentMethod string
	currentMethod, err = getAPMInjectionMethod()
	if err != nil {
		return fmt.Errorf("could not get current injection method: %w", err)
	}

	// Check if a different injection method is configured for this installation we should first uninstrument the current method
	envInst := env.FromEnv()
	newMethod := envInst.InstallScript.APMInstrumentationEnabled

	fmt.Printf("currentMethod: %s, newMethod: %s\n", currentMethod, newMethod)

	if currentMethod == env.APMInstrumentationNotSet {
		if newMethod != env.APMInstrumentationNotSet {
			return instrumentDotnetLibrary(ctx, newMethod, target)
		} else {
			return nil
		}

	}

	if newMethod != env.APMInstrumentationNotSet && newMethod != currentMethod {
		err = uninstrumentDotnetLibrary(ctx, currentMethod, target)
		if err != nil {
			log.Errorf("Error changing instrumentation method for dotnet library, could not uninstrument the current method (%s): %v", currentMethod, err)
		}
	} else {
		newMethod = currentMethod
	}

	return instrumentDotnetLibrary(ctx, newMethod, target)
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
		return fmt.Errorf("unsupported injection method: %s", method)
	}
}

func uninstrumentDotnetLibraryIfNeeded(ctx context.Context, target string) (err error) {
	var method string
	method, err = getAPMInjectionMethod()
	if err != nil {
		return err
	}
	if method == env.APMInstrumentationNotSet {
		return nil
	}
	return uninstrumentDotnetLibrary(ctx, method, target)
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
		return fmt.Errorf("unsupported injection method: %s", method)
	}
}
