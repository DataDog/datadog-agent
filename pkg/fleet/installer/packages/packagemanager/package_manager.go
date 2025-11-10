// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package packagemanager provides an interface over the OS package manager
package packagemanager

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

func dpkgInstalled() (bool, error) {
	_, err := exec.LookPath("dpkg")
	if err != nil && !errors.Is(err, exec.ErrNotFound) {
		return false, err
	}
	return err == nil, nil
}

func rpmInstalled() (bool, error) {
	_, err := exec.LookPath("rpm")
	if err != nil && !errors.Is(err, exec.ErrNotFound) {
		return false, err
	}
	return err == nil, nil
}

// RemovePackage removes a package installed via deb/rpm package manager
// It doesn't remove dependencies or purge as we want to keep existing configuration files
// and reinstall the package using the installer.
func RemovePackage(ctx context.Context, pkg string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "RemovePackage")
	defer func() { span.Finish(err) }()

	dpkgInstalled, err := dpkgInstalled()
	if err != nil {
		return err
	}
	rpmInstalled, err := rpmInstalled()
	if err != nil {
		return err
	}
	var packageInstalled bool
	var removeCmd *exec.Cmd
	if dpkgInstalled {
		removeCmd = exec.Command("dpkg", "-r", pkg)
		packageInstalled = exec.Command("dpkg", "-s", pkg).Run() == nil
	}
	if rpmInstalled {
		removeCmd = exec.Command("rpm", "-e", pkg)
		packageInstalled = exec.Command("rpm", "-q", pkg).Run() == nil
	}
	if !packageInstalled {
		return nil
	}
	out, err := removeCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to uninstall deb/rpm package %s (%w): %s", pkg, err, out)
	}
	return nil
}
