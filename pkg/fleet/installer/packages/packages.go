// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packages contains the install/upgrades/uninstall logic for packages
package packages

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

type packageHook func(ctx HookContext) error

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

	postStartConfigExperiment   packageHook
	preStopConfigExperiment     packageHook
	postPromoteConfigExperiment packageHook
}

// Hooks is the interface for the hooks.
type Hooks interface {
	PreInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) error
	PreRemove(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) error
	PostInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool, winArgs []string) error

	PreStartExperiment(ctx context.Context, pkg string) error
	PostStartExperiment(ctx context.Context, pkg string) error
	PreStopExperiment(ctx context.Context, pkg string) error
	PostStopExperiment(ctx context.Context, pkg string) error
	PrePromoteExperiment(ctx context.Context, pkg string) error
	PostPromoteExperiment(ctx context.Context, pkg string) error

	PostStartConfigExperiment(ctx context.Context, pkg string) error
	PreStopConfigExperiment(ctx context.Context, pkg string) error
	PostPromoteConfigExperiment(ctx context.Context, pkg string) error
}

// NewHooks creates a new Hooks instance that will execute hooks via the CLI.
func NewHooks(env *env.Env, packages *repository.Repositories) Hooks {
	return &hooksCLI{
		env:      env,
		packages: packages,
	}
}

type hooksCLI struct {
	env      *env.Env
	packages *repository.Repositories
}

// PreInstall calls the pre-install hook for the package.
func (h *hooksCLI) PreInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) error {
	return h.callHook(ctx, false, pkg, "preInstall", pkgType, upgrade, nil)
}

// PreRemove calls the pre-remove hook for the package.
func (h *hooksCLI) PreRemove(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) error {
	return h.callHook(ctx, false, pkg, "preRemove", pkgType, upgrade, nil)
}

// PostInstall calls the post-install hook for the package.
func (h *hooksCLI) PostInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool, winArgs []string) error {
	return h.callHook(ctx, false, pkg, "postInstall", pkgType, upgrade, winArgs)
}

// PreStartExperiment calls the pre-start-experiment hook for the package.
func (h *hooksCLI) PreStartExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, false, pkg, "preStartExperiment", PackageTypeOCI, false, nil)
}

// PostStartExperiment calls the post-start-experiment hook for the package.
func (h *hooksCLI) PostStartExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, true, pkg, "postStartExperiment", PackageTypeOCI, false, nil)
}

// PreStopExperiment calls the pre-stop-experiment hook for the package.
func (h *hooksCLI) PreStopExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, true, pkg, "preStopExperiment", PackageTypeOCI, false, nil)
}

// PostStopExperiment calls the post-stop-experiment hook for the package.
func (h *hooksCLI) PostStopExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, false, pkg, "postStopExperiment", PackageTypeOCI, false, nil)
}

// PrePromoteExperiment calls the pre-promote-experiment hook for the package.
func (h *hooksCLI) PrePromoteExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, false, pkg, "prePromoteExperiment", PackageTypeOCI, false, nil)
}

// PostPromoteExperiment calls the post-promote-experiment hook for the package.
func (h *hooksCLI) PostPromoteExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, true, pkg, "postPromoteExperiment", PackageTypeOCI, false, nil)
}

// PostStartConfigExperiment calls the post-start-config-experiment hook for the package.
func (h *hooksCLI) PostStartConfigExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, false, pkg, "postStartConfigExperiment", PackageTypeOCI, false, nil)
}

// PreStopConfigExperiment calls the pre-stop-config-experiment hook for the package.
func (h *hooksCLI) PreStopConfigExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, false, pkg, "preStopConfigExperiment", PackageTypeOCI, false, nil)
}

// PostPromoteConfigExperiment calls the post-promote-config-experiment hook for the package.
func (h *hooksCLI) PostPromoteConfigExperiment(ctx context.Context, pkg string) error {
	return h.callHook(ctx, false, pkg, "postPromoteConfigExperiment", PackageTypeOCI, false, nil)
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

// HookContext is the context passed to hooks during install/upgrade/uninstall.
type HookContext struct {
	context.Context `json:"-"`
	Package         string      `json:"package"`
	PackageType     PackageType `json:"package_type"`
	PackagePath     string      `json:"package_path"`
	Hook            string      `json:"hook"`
	Upgrade         bool        `json:"upgrade"`
	WindowsArgs     []string    `json:"windows_args"`
}

// StartSpan starts a new span with the given operation name.
func (c HookContext) StartSpan(operationName string) (*telemetry.Span, HookContext) {
	span, newCtx := telemetry.StartSpanFromContext(c, operationName)
	span.SetTag("package", c.Package)
	span.SetTag("package_type", c.PackageType)
	span.SetTag("package_path", c.PackagePath)
	span.SetTag("upgrade", c.Upgrade)
	span.SetTag("windows_args", c.WindowsArgs)
	c.Context = newCtx
	return span, c
}

func (h *hooksCLI) getPath(pkg string, pkgType PackageType, experiment bool) string {
	switch pkgType {
	case PackageTypeOCI:
		switch experiment {
		case false:
			return h.packages.Get(pkg).StablePath()
		case true:
			return h.packages.Get(pkg).ExperimentPath()
		}
	case PackageTypeDEB, PackageTypeRPM:
		if pkg == "datadog-agent" {
			return "/opt/datadog-agent"
		}
	}
	panic(fmt.Sprintf("unknown package type with package: %s, %s", pkgType, pkg))
}

func (h *hooksCLI) callHook(ctx context.Context, experiment bool, pkg string, name string, packageType PackageType, upgrade bool, windowsArgs []string) error {
	hooksCLIPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	pkgPath := h.getPath(pkg, packageType, experiment)
	if pkg == "datadog-agent" && runtime.GOOS == "linux" && name != "preInstall" {
		agentInstallerPath := filepath.Join(pkgPath, "embedded", "bin", "installer")
		_, err := os.Stat(agentInstallerPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to check if agent installer exists at (%s): %w", agentInstallerPath, err)
		}
		if !os.IsNotExist(err) {
			hooksCLIPath = agentInstallerPath
		}
	}
	hookCtx := HookContext{
		Context:     ctx,
		Hook:        name,
		Package:     pkg,
		PackagePath: pkgPath,
		PackageType: packageType,
		Upgrade:     upgrade,
		WindowsArgs: windowsArgs,
	}
	serializedHookCtx, err := json.Marshal(hookCtx)
	if err != nil {
		return fmt.Errorf("failed to serialize hook context: %w", err)
	}
	// FIXME: remove when we drop support for the installer
	if pkg == "datadog-installer" {
		return RunHook(hookCtx)
	}
	i := exec.NewInstallerExec(h.env, hooksCLIPath)
	err = i.RunHook(ctx, string(serializedHookCtx))
	if err != nil {
		return fmt.Errorf("failed to run hook (%s): %w", name, err)
	}
	return nil
}

// RunHook executes a hook for a package
func RunHook(ctx HookContext) (err error) {
	hook := getHook(ctx.Package, ctx.Hook)
	if hook == nil {
		span, ok := telemetry.SpanFromContext(ctx)
		if ok {
			span.SetTag("unknown_hook", true)
		}
		return nil
	}
	span, hookCtx := ctx.StartSpan(fmt.Sprintf("package.%s.%s", ctx.Package, ctx.Hook))
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
	case "postStartConfigExperiment":
		return h.postStartConfigExperiment
	case "preStopConfigExperiment":
		return h.preStopConfigExperiment
	case "postPromoteConfigExperiment":
		return h.postPromoteConfigExperiment
	}
	return nil
}

// PackageCommandHandler is a function that handles the execution of a package-specific command.
//
// Implement this function and add it to the packagesCommands map to enable package-specific commands
// for a given package. Package commands are currently intended to be used internally by package hooks
// and not exposed to the user.
// For example, the Agent Windows package hooks must start some background worker processes.
//
// The content of the command string is entirely defined by the individual package. Do NOT include
// private information in the command string, use environment variables instead.
type PackageCommandHandler func(ctx context.Context, command string) error

// RunPackageCommand runs a package-specific command
func RunPackageCommand(ctx context.Context, packageName string, command string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, fmt.Sprintf("package.%s", packageName))
	span.SetTag("command", command)
	defer func() { span.Finish(err) }()

	// Get the command handler for this package
	handler, ok := packageCommands[packageName]
	if !ok {
		return fmt.Errorf("no command handler found for package: %s", packageName)
	}

	// Call the package-specific command handler
	return handler(ctx, command)
}
