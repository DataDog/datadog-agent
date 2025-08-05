// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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
	_, err = dotnetExec.EnableIISInstrumentation(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	err = copyIISInstrumentationScript(ctx)
	if err != nil {
		log.Errorf("Failed to copy iis instrumentation script: %v", err)
	}
	return nil
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
	_, err = dotnetExec.EnableIISInstrumentation(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	err = copyIISInstrumentationScript(ctx)
	if err != nil {
		log.Errorf("Failed to copy iis instrumentation script: %v", err)
	}
	return nil
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
	_, err = dotnetExec.EnableIISInstrumentation(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	err = copyIISInstrumentationScript(ctx)
	if err != nil {
		log.Errorf("Failed to copy iis instrumentation script: %v", err)
	}
	return nil
}

// preRemoveAPMLibraryDotnet uninstalls the .NET APM library
// This function only disable injection, the cleanup for each version is done by the PreRemoveHook
func preRemoveAPMLibraryDotnet(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("remove_apm_library_dotnet")
	defer func() { span.Finish(err) }()
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("stable"))
	if err != nil {
		// If the remove is being retried after a failed first attempt, the stable symlink may have been removed
		// so we do not consider this an error
		if errors.Is(err, fs.ErrNotExist) {
			log.Warn("Stable symlink does not exist, assuming the package has already been partially removed and skipping UninstallProduct")
			return nil
		}
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	_, err = dotnetExec.RemoveIISInstrumentation(ctx)
	if err != nil {
		return err
	}
	return nil
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

func writeBytesToFile(content []byte, dst string) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), "")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	_, err = tmp.Write(content)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}

	err = tmp.Sync()
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()

	err = os.Rename(tmpName, dst)
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	return nil
}

func copyIISInstrumentationScript(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "copy_iis_instrumentation_script")
	defer func() { span.Finish(err) }()
	dst := filepath.Join(paths.RunPath, "iis-instrumentation.bat")
	err = writeBytesToFile(embedded.ScriptIISInstrumentation, dst)
	return err
}

func UpdateIISScriptIfNeeded(ctx context.Context) (err error) {
	db, err := db.New(filepath.Join(paths.PackagesPath, "packages.db"), db.WithTimeout(10*time.Second))
	if err != nil {
		return err
	}
	ok, err := db.HasPackage("datadog-apm-library-dotnet")
	if err != nil {
		return err
	}
	if ok {
		return copyIISInstrumentationScript(ctx)
	}
	return nil
}
