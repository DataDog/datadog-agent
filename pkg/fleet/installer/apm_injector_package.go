// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service"
)

type apmInjectorPackageInstaller struct {
	*basePackageInstaller
}

func (a *apmInjectorPackageInstaller) SetupPackage(ctx context.Context, version, sourcePath string, args []string) error {
	err := a.basePackageInstaller.SetupPackage(ctx, version, sourcePath, args)
	if err != nil {
		return err
	}
	return service.SetupAPMInjector(ctx)
}

func (a *apmInjectorPackageInstaller) RemovePackage(ctx context.Context) error {
	err := a.basePackageInstaller.RemovePackage(ctx)
	if err != nil {
		return err
	}
	return service.RemoveAPMInjector(ctx)
}
