// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packages contains the install/upgrades/uninstall logic for packages
package packages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

type packageHook func(ctx hookContext) error

// hooks represents the hooks for a package.
type hooks struct {
	preInstall  packageHook
	preRemove   packageHook
	postInstall packageHook

	preStartExperiment    packageHook
	postStartExperiment   packageHook
	preStopExperiment     packageHook
	postStopExperiment    packageHook
	prePromoteExperiment  packageHook
	postPromoteExperiment packageHook
}

// PreInstall calls the pre-install hook for the package.
func PreInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) error {
	return callHook(ctx, env.FromEnv(), false, pkg, "preInstall", pkgType, upgrade, nil)
}

// PreRemove calls the pre-remove hook for the package.
func PreRemove(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) error {
	return callHook(ctx, env.FromEnv(), false, pkg, "preRemove", pkgType, upgrade, nil)
}

// PostInstall calls the post-install hook for the package.
func PostInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool, winArgs []string) error {
	return callHook(ctx, env.FromEnv(), false, pkg, "postInstall", pkgType, upgrade, winArgs)
}

// PreStartExperiment calls the pre-start-experiment hook for the package.
func PreStartExperiment(ctx context.Context, pkg string) error {
	return callHook(ctx, env.FromEnv(), false, pkg, "preStartExperiment", PackageTypeOCI, false, nil)
}

// PostStartExperiment calls the post-start-experiment hook for the package.
func PostStartExperiment(ctx context.Context, pkg string) error {
	return callHook(ctx, env.FromEnv(), true, pkg, "postStartExperiment", PackageTypeOCI, false, nil)
}

// PreStopExperiment calls the pre-stop-experiment hook for the package.
func PreStopExperiment(ctx context.Context, pkg string) error {
	return callHook(ctx, env.FromEnv(), true, pkg, "preStopExperiment", PackageTypeOCI, false, nil)
}

// PostStopExperiment calls the post-stop-experiment hook for the package.
func PostStopExperiment(ctx context.Context, pkg string) error {
	return callHook(ctx, env.FromEnv(), false, pkg, "postStopExperiment", PackageTypeOCI, false, nil)
}

// PrePromoteExperiment calls the pre-promote-experiment hook for the package.
func PrePromoteExperiment(ctx context.Context, pkg string) error {
	return callHook(ctx, env.FromEnv(), true, pkg, "prePromoteExperiment", PackageTypeOCI, false, nil)
}

// PostPromoteExperiment calls the post-promote-experiment hook for the package.
func PostPromoteExperiment(ctx context.Context, pkg string) error {
	return callHook(ctx, env.FromEnv(), false, pkg, "postPromoteExperiment", PackageTypeOCI, false, nil)
}

// PackageType is the type of package.
type PackageType string

const (
	// PackageTypeOCI is the type for OCI packages.
	PackageTypeOCI PackageType = "oci"
	// PackageTypeDEB is the type for DEB packages.
	PackageTypeDEB PackageType = "deb"
	// PackageTypeRPM is the type for RPM packages.
	PackageTypeRPM PackageType = "rpm"
)

// hookContext is the context passed to hooks during install/upgrade/uninstall.
type hookContext struct {
	context.Context
	Package     string
	Type        PackageType
	Upgrade     bool
	Path        string
	WindowsArgs []string
}

// StartSpan starts a new span with the given operation name.
func (c hookContext) StartSpan(operationName string) (*telemetry.Span, hookContext) {
	span, newCtx := telemetry.StartSpanFromContext(c, operationName)
	span.SetTag("package", c.Package)
	span.SetTag("type", c.Type)
	span.SetTag("upgrade", c.Upgrade)
	span.SetTag("path", c.Path)
	span.SetTag("windows_args", c.WindowsArgs)
	c.Context = newCtx
	return span, c
}

func getPath(pkg string, pkgType PackageType, experiment bool) string {
	switch pkgType {
	case PackageTypeOCI:
		symlink := "stable"
		if experiment {
			symlink = "experiment"
		}
		return filepath.Join(paths.PackagesPath, pkg, symlink)
	case PackageTypeDEB, PackageTypeRPM:
		if pkg == "datadog-agent" {
			return "/opt/datadog-agent"
		}
	}
	panic(fmt.Sprintf("unknown package type with package: %s, %s", pkgType, pkg))
}

func callHook(ctx context.Context, env *env.Env, experiment bool, pkg string, name string, packageType PackageType, upgrade bool, windowsArgs []string) error {
	hooksCLIPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	if pkg == "datadog-agent" && runtime.GOOS == "linux" && name != "preInstall" {
		hooksCLIPath = filepath.Join(getPath(pkg, packageType, experiment), "embedded", "bin", "installer")
		if _, err := os.Stat(hooksCLIPath); os.IsNotExist(err) {
			return fmt.Errorf("failed to find installer to run hooks at (%s): %w", hooksCLIPath, err)
		}
	}
	i := exec.NewInstallerExec(env, hooksCLIPath)
	err = i.RunHook(ctx, pkg, name, string(packageType), upgrade, windowsArgs)
	if err != nil {
		return fmt.Errorf("failed to run hook (%s): %w", name, err)
	}
	return nil
}

// RunHook executes a hook for a package
func RunHook(ctx context.Context, pkg string, name string, packageType PackageType, upgrade bool, windowsArgs []string) (err error) {
	hook := getHook(pkg, name)
	if hook == nil {
		return nil
	}
	hookCtx := hookContext{
		Context:     ctx,
		Package:     pkg,
		Type:        packageType,
		Upgrade:     upgrade,
		WindowsArgs: windowsArgs,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.%s", pkg, name))
	defer func() { span.Finish(err) }()
	return hook(hookCtx)
}

func getHook(pkg string, name string) packageHook {
	h := packagesHooks[pkg]
	switch name {
	case "postInstall":
		return h.postInstall
	case "preRemove":
		return h.preRemove
	case "preInstall":
		return h.preInstall
	case "preStartExperiment":
		return h.preStartExperiment
	case "postStartExperiment":
		return h.postStartExperiment
	case "preStopExperiment":
		return h.preStopExperiment
	case "postStopExperiment":
		return h.postStopExperiment
	case "prePromoteExperiment":
		return h.prePromoteExperiment
	case "postPromoteExperiment":
		return h.postPromoteExperiment
	}
	return nil
}
