// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service"
)

type datadogInstallerPackageInstaller struct {
	*basePackageInstaller
}

func (d *datadogInstallerPackageInstaller) StartExperiment(ctx context.Context, version, sourcePath string) error {
	err := d.basePackageInstaller.StartExperiment(ctx, version, sourcePath)
	if err != nil {
		return err
	}
	return service.StartInstallerExperiment(ctx)
}

func (d *datadogInstallerPackageInstaller) StopExperiment(ctx context.Context) error {
	err := d.basePackageInstaller.StopExperiment(ctx)
	if err != nil {
		return err
	}
	// In the Datadog Installer this call will kill the current process, so we want to delete from the repository first.
	return service.StopInstallerExperiment(ctx)
}

func (d *datadogInstallerPackageInstaller) PromoteExperiment(ctx context.Context) error {
	err := d.basePackageInstaller.PromoteExperiment(ctx)
	if err != nil {
		return err
	}
	return service.StopInstallerExperiment(ctx)
}

func (d *datadogInstallerPackageInstaller) SetupPackage(ctx context.Context, version, sourcePath string, args []string) error {
	err := d.basePackageInstaller.SetupPackage(ctx, version, sourcePath, args)
	if err != nil {
		return err
	}
	return service.SetupInstaller(ctx)
}

func (d *datadogInstallerPackageInstaller) RemovePackage(ctx context.Context) error {
	err := d.basePackageInstaller.RemovePackage(ctx)
	if err != nil {
		return err
	}
	return service.RemoveInstaller(ctx)
}
