// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service"
)

type datadogAgentPackageInstaller struct {
	*basePackageInstaller
}

func (d *datadogAgentPackageInstaller) StartExperiment(ctx context.Context, version, sourcePath string) error {
	err := d.basePackageInstaller.StartExperiment(ctx, version, sourcePath)
	if err != nil {
		return err
	}
	return service.StartAgentExperiment(ctx)
}

func (d *datadogAgentPackageInstaller) StopExperiment(ctx context.Context) error {
	err := service.StopAgentExperiment(ctx)
	if err != nil {
		return err
	}
	// Delete the experiment from the repository *after* we have stopped the experiment
	return d.basePackageInstaller.StopExperiment(ctx)
}

func (d *datadogAgentPackageInstaller) PromoteExperiment(ctx context.Context) error {
	err := d.basePackageInstaller.PromoteExperiment(ctx)
	if err != nil {
		return err
	}
	return service.PromoteAgentExperiment(ctx)
}

func (d *datadogAgentPackageInstaller) SetupPackage(ctx context.Context, version, sourcePath string, args []string) error {
	err := d.basePackageInstaller.SetupPackage(ctx, version, sourcePath, args)
	if err != nil {
		return err
	}
	return service.SetupAgent(ctx, args)
}

func (d *datadogAgentPackageInstaller) RemovePackage(ctx context.Context) error {
	err := d.basePackageInstaller.RemovePackage(ctx)
	if err != nil {
		return err
	}
	return service.RemoveAgent(ctx)
}
