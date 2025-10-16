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
	"golang.org/x/sys/windows/registry"
)

var apmLibraryDotnetPackage = hooks{
	postInstall:         postInstallAPMLibraryDotnet,
	preRemove:           preRemoveAPMLibraryDotnet,
	postStartExperiment: postStartExperimentAPMLibraryDotnet,
	preStopExperiment:   preStopExperimentAPMLibraryDotnet,
}

const (
	packageAPMLibraryDotnet        = "datadog-apm-library-dotnet"
	apmRegistryKey                 = `SOFTWARE\Datadog\Datadog Installer\APM`
	dotnetInstrumentationMethodKey = "DotnetInstrumentationMode"
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
// This function only disable injection, the cleanup for each version is done by the asyncPreRemoveHook
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
	err = unsetDotnetInstrumentationMethod()
	if err != nil {
		log.Error("Unable to unset the instrumentation method")
	}
	return uninstrumentDotnetLibrary(ctx.Context, "stable")
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

func instrumentDotnetLibrary(ctx context.Context, target string) (err error) {
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath(target))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	err = setDotnetInstrumentationMethod(env.APMInstrumentationEnabledIIS)
	if err != nil {
		return fmt.Errorf("Unable to set instrumentation method: %w", err)
	}
	_, err = dotnetExec.EnableIISInstrumentation(ctx, getLibraryPath(installDir))
	return err
}

func uninstrumentDotnetLibrary(ctx context.Context, target string) (err error) {
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath(target))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	_, err = dotnetExec.RemoveIISInstrumentation(ctx)
	if err != nil {
		return err
	}
	err = setDotnetInstrumentationMethod(env.APMInstrumentationDisabled)
	return fmt.Errorf("Unable to set instrumentation method: %w", err)
}

func instrumentDotnetLibraryIfNeeded(ctx context.Context, target string, isUpgrade bool) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrumentDotnetLibraryIfNeeded")
	defer func() { span.Finish(err) }()

	envInst := env.FromEnv()
	newMethod := envInst.InstallScript.APMInstrumentationEnabled
	var currentMethod string
	currentMethod, err = getDotnetInstrumentationMethod()
	if err != nil {
		return fmt.Errorf("could not get current instrumentation method: %w", err)
	}

	// Older versions of the agent did not store the instrumentation method and always enabled IIS instrumentation
	// So if it's an upgrade and not a reinstall, we assume IIS instrumentation is enabled
	upgradeFromOldVersion := (currentMethod == env.APMInstrumentationNotSet && isUpgrade)

	if (newMethod == env.APMInstrumentationEnabledIIS) || (currentMethod == env.APMInstrumentationEnabledIIS) || upgradeFromOldVersion {
		if err = instrumentDotnetLibrary(ctx, target); err != nil {
			return fmt.Errorf("could not instrument dotnet library: %w", err)
		}
	} else {
		return setDotnetInstrumentationMethod(env.APMInstrumentationDisabled)
	}
	return nil
}

func setDotnetInstrumentationMethod(method string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, apmRegistryKey, registry.ALL_ACCESS)
	if err != nil {
		// If the key doesn't exist, create it
		k, _, err = registry.CreateKey(registry.LOCAL_MACHINE, apmRegistryKey, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
	}
	defer k.Close()

	return k.SetStringValue(dotnetInstrumentationMethodKey, method)
}

func unsetDotnetInstrumentationMethod() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, apmRegistryKey, registry.ALL_ACCESS)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()

	return k.DeleteValue(apmRegistryKey)
}

func getDotnetInstrumentationMethod() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, apmRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return env.APMInstrumentationNotSet, nil
		}
		return "", err
	}
	defer k.Close()

	method, _, err := k.GetStringValue(dotnetInstrumentationMethodKey)
	if err != nil {
		if err == registry.ErrNotExist {
			return env.APMInstrumentationNotSet, nil
		}
		return "", err
	}
	return method, nil
}
