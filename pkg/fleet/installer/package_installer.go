// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import "context"

type packageInstaller interface {
	StartExperiment(ctx context.Context, version, sourcePath string) error
	StopExperiment(ctx context.Context) error
	PromoteExperiment(ctx context.Context) error
	SetupPackage(ctx context.Context, version, sourcePath string, args []string) error
	RemovePackage(ctx context.Context) error
}

type basePackageInstaller struct {
	installerImpl *installerImpl
	pkgName       string
}

func (i *basePackageInstaller) StartExperiment(ctx context.Context, version, sourcePath string) error {
	repository := i.installerImpl.repositories.Get(i.pkgName)
	return repository.SetExperiment(ctx, version, sourcePath)
}

func (i *basePackageInstaller) StopExperiment(ctx context.Context) error {
	repository := i.installerImpl.repositories.Get(i.pkgName)
	return repository.DeleteExperiment(ctx)
}

func (i *basePackageInstaller) PromoteExperiment(ctx context.Context) error {
	repository := i.installerImpl.repositories.Get(i.pkgName)
	return repository.PromoteExperiment(ctx)
}

func (i *basePackageInstaller) SetupPackage(ctx context.Context, version, sourcePath string, _ []string) error {
	return i.installerImpl.repositories.Create(ctx, i.pkgName, version, sourcePath)
}

func (i *basePackageInstaller) RemovePackage(context.Context) error {
	return nil
}
