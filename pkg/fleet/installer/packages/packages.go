// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packages contains the install/upgrades/uninstall logic for packages
package packages

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

type packageHook func(ctx PackageContext) error

// Package represents a package that can be installed, upgraded, or removed.
type Package struct {
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

	// AsyncPreRemoveHook is called before a package is removed from the disk.
	// It can block the removal of the package files until a condition is met without blocking
	// the rest of the uninstall or upgrade process.
	// Today this is only useful for the dotnet tracer on windows and generally *SHOULD BE AVOIDED*.
	AsyncPreRemoveHook repository.PreRemoveHook
}

// PackageHook is the interface for package hooks.
type PackageHook interface {
	PreInstall(ctx PackageContext) (err error)
	PreRemove(ctx PackageContext) (err error)
	PostInstall(ctx PackageContext) (err error)
	PreStartExperiment(ctx PackageContext) (err error)
	PostStartExperiment(ctx PackageContext) (err error)
	PreStopExperiment(ctx PackageContext) (err error)
	PostStopExperiment(ctx PackageContext) (err error)
	PrePromoteExperiment(ctx PackageContext) (err error)
	PostPromoteExperiment(ctx PackageContext) (err error)
}

// PreInstall calls the pre-install hook for the package.
func (p Package) PreInstall(ctx PackageContext) (err error) {
	if p.preInstall == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.preInstall", p.name))
	defer func() { span.Finish(err) }()
	return p.preInstall(ctx)
}

// PreRemove calls the pre-remove hook for the package.
func (p Package) PreRemove(ctx PackageContext) (err error) {
	if p.preRemove == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.preRemove", p.name))
	defer func() { span.Finish(err) }()
	return p.preRemove(ctx)
}

// PostInstall calls the post-install hook for the package.
func (p Package) PostInstall(ctx PackageContext) (err error) {
	if p.postInstall == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.postInstall", p.name))
	defer func() { span.Finish(err) }()
	return p.postInstall(ctx)
}

// PreStartExperiment calls the pre-start-experiment hook for the package.
func (p Package) PreStartExperiment(ctx PackageContext) (err error) {
	if p.preStartExperiment == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.preStartExperiment", p.name))
	defer func() { span.Finish(err) }()
	return p.preStartExperiment(ctx)
}

// PostStartExperiment calls the post-start-experiment hook for the package.
func (p Package) PostStartExperiment(ctx PackageContext) (err error) {
	if p.postStartExperiment == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.postStartExperiment", p.name))
	defer func() { span.Finish(err) }()
	return p.postStartExperiment(ctx)
}

// PreStopExperiment calls the pre-stop-experiment hook for the package.
func (p Package) PreStopExperiment(ctx PackageContext) (err error) {
	if p.preStopExperiment == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.preStopExperiment", p.name))
	defer func() { span.Finish(err) }()
	return p.preStopExperiment(ctx)
}

// PostStopExperiment calls the post-stop-experiment hook for the package.
func (p Package) PostStopExperiment(ctx PackageContext) (err error) {
	if p.postStopExperiment == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.postStopExperiment", p.name))
	defer func() { span.Finish(err) }()
	return p.postStopExperiment(ctx)
}

// PrePromoteExperiment calls the pre-promote-experiment hook for the package.
func (p Package) PrePromoteExperiment(ctx PackageContext) (err error) {
	if p.prePromoteExperiment == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.prePromoteExperiment", p.name))
	defer func() { span.Finish(err) }()
	return p.prePromoteExperiment(ctx)
}

// PostPromoteExperiment calls the post-promote-experiment hook for the package.
func (p Package) PostPromoteExperiment(ctx PackageContext) (err error) {
	if p.postPromoteExperiment == nil {
		return nil
	}
	span, ctx := ctx.StartSpan(fmt.Sprintf("package.%s.postPromoteExperiment", p.name))
	defer func() { span.Finish(err) }()
	return p.postPromoteExperiment(ctx)
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

// PackageContext is the context passed to hooks during install/upgrade/uninstall.
type PackageContext struct {
	context.Context
	Package string
	Type    PackageType
	Upgrade bool
	Path    string
	Args    []string
}

// StartSpan starts a new span with the given operation name.
func (c PackageContext) StartSpan(operationName string) (*telemetry.Span, PackageContext) {
	span, newCtx := telemetry.StartSpanFromContext(c, operationName)
	span.SetTag("package", c.Package)
	span.SetTag("type", c.Type)
	span.SetTag("upgrade", c.Upgrade)
	span.SetTag("path", c.Path)
	span.SetTag("args", c.Args)
	c.Context = newCtx
	return span, c
}
