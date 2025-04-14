// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packages contains the install/upgrades/uninstall logic for packages
package packages

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

type packageHook func(ctx hookContext) error

// hooks represents the hooks for a package.
type hooks struct {
	name string

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

// // Hooks is the interface for package hooks.
// type Hooks interface {
// 	PreInstall(ctx context.Context, pkgType PackageType, upgrade bool) (err error)
// 	PreRemove(ctx context.Context, pkgType PackageType, upgrade bool) (err error)
// 	PostInstall(ctx context.Context, pkgType PackageType, upgrade bool, winArgs []string) (err error)
// 	PreStartExperiment(ctx context.Context) (err error)
// 	PostStartExperiment(ctx context.Context) (err error)
// 	PreStopExperiment(ctx context.Context) (err error)
// 	PostStopExperiment(ctx context.Context) (err error)
// 	PrePromoteExperiment(ctx context.Context) (err error)
// 	PostPromoteExperiment(ctx context.Context) (err error)
// }

func (h *hooks) getPath(pkg string, pkgType PackageType, experiment bool) (string, error) {
	switch pkgType {
	case PackageTypeOCI:
		symlink := "stable"
		if experiment {
			symlink = "experiment"
		}
		return filepath.Join(paths.PackagesPath, pkg, symlink), nil
	case PackageTypeDEB, PackageTypeRPM:
		if pkg == "datadog-agent" {
			return "/opt/datadog-agent", nil
		}
	}
	return "", fmt.Errorf("unknown package or package type: %s, %s", pkg, pkgType)
}

// PreInstall calls the pre-install hook for the package.
func PreInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) (err error) {
	if h.preInstall == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: h.name,
		Type:    pkgType,
		Upgrade: upgrade,
		Path:    path,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.preInstall", h.name))
	defer func() { span.Finish(err) }()
	return h.preInstall(hookCtx)
}

// PreRemove calls the pre-remove hook for the package.
func PreRemove(ctx context.Context, pkg string, pkgType PackageType, upgrade bool) (err error) {
	if h.preRemove == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Type:    pkgType,
		Upgrade: upgrade,
		Path:    path,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.preRemove", h.name))
	defer func() { span.Finish(err) }()
	return h.preRemove(hookCtx)
}

// PostInstall calls the post-install hook for the package.
func PostInstall(ctx context.Context, pkg string, pkgType PackageType, upgrade bool, winArgs []string) (err error) {
	if h.postInstall == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Type:    pkgType,
		Upgrade: upgrade,
		Path:    path,
		Args:    winArgs,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.postInstall", pkg))
	defer func() { span.Finish(err) }()
	return h.postInstall(hookCtx)
}

// PreStartExperiment calls the pre-start-experiment hook for the package.
func PreStartExperiment(ctx context.Context, pkg string) (err error) {
	if h.preStartExperiment == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Path:    path,
		Type:    PackageTypeOCI,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.preStartExperiment", pkg))
	defer func() { span.Finish(err) }()
	return h.preStartExperiment(hookCtx)
}

// PostStartExperiment calls the post-start-experiment hook for the package.
func PostStartExperiment(ctx context.Context, pkg string) (err error) {
	if h.postStartExperiment == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Path:    path,
		Type:    PackageTypeOCI,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.postStartExperiment", pkg))
	defer func() { span.Finish(err) }()
	return h.postStartExperiment(hookCtx)
}

// PreStopExperiment calls the pre-stop-experiment hook for the package.
func PreStopExperiment(ctx context.Context, pkg string) (err error) {
	if h.preStopExperiment == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Path:    path,
		Type:    PackageTypeOCI,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.preStopExperiment", pkg))
	defer func() { span.Finish(err) }()
	return h.preStopExperiment(hookCtx)
}

// PostStopExperiment calls the post-stop-experiment hook for the package.
func PostStopExperiment(ctx context.Context, pkg string) (err error) {
	if h.postStopExperiment == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Path:    path,
		Type:    PackageTypeOCI,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.postStopExperiment", pkg))
	defer func() { span.Finish(err) }()
	return h.postStopExperiment(hookCtx)
}

// PrePromoteExperiment calls the pre-promote-experiment hook for the package.
func PrePromoteExperiment(ctx context.Context, pkg string) (err error) {
	if h.prePromoteExperiment == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Path:    path,
		Type:    PackageTypeOCI,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.prePromoteExperiment", pkg))
	defer func() { span.Finish(err) }()
	return h.prePromoteExperiment(hookCtx)
}

// PostPromoteExperiment calls the post-promote-experiment hook for the package.
func PostPromoteExperiment(ctx context.Context, pkg string) (err error) {
	if h.postPromoteExperiment == nil {
		return nil
	}
	hookCtx := hookContext{
		Context: ctx,
		Package: pkg,
		Path:    path,
		Type:    PackageTypeOCI,
	}
	span, hookCtx := hookCtx.StartSpan(fmt.Sprintf("package.%s.postPromoteExperiment", pkg))
	defer func() { span.Finish(err) }()
	return h.postPromoteExperiment(hookCtx)
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
	Package string
	Type    PackageType
	Upgrade bool
	Path    string
	Args    []string
}

// StartSpan starts a new span with the given operation name.
func (c hookContext) StartSpan(operationName string) (*telemetry.Span, hookContext) {
	span, newCtx := telemetry.StartSpanFromContext(c, operationName)
	span.SetTag("package", c.Package)
	span.SetTag("type", c.Type)
	span.SetTag("upgrade", c.Upgrade)
	span.SetTag("path", c.Path)
	span.SetTag("args", c.Args)
	c.Context = newCtx
	return span, c
}
