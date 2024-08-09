// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"path/filepath"
)

const (
	datadogInstaller = "datadog-installer"
)

// SetupInstaller installs and starts the installer
func SetupInstaller(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "setup_installer")
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup installer: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	err = msiexec("stable", datadogInstaller, "/i", nil)
	return err
}

// RemoveInstaller noop
func RemoveInstaller(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "remove_installer")
	defer func() {
		if err != nil {
			log.Errorf("Failed to remove installer: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	targets := []string{
		"experiment",
		"stable",
	}
	for _, target := range targets {
		updaterPath := filepath.Join(paths.PackagesPath, datadogInstaller, target)
		msis, findErr := filepath.Glob(filepath.Join(updaterPath, fmt.Sprintf("%s-*-1-x86_64.msi", datadogInstaller)))
		if len(msis) == 1 && findErr == nil {
			err = msiexec(target, datadogInstaller, "/x", nil)
			if err == nil {
				// There can only be one Installer per system.
				// If we successfully uninstalled, there is nothing else to do.
				break
			}
		} else {
			log.Infof("%s installer not found, skipping (%v)", target, findErr)
		}
	}

	return err
}

// StartInstallerExperiment starts the installer experiment
func StartInstallerExperiment(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "start_installer_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to start installer experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	err = msiexec("experiment", datadogInstaller, "/i", nil)
	return err
}

// StopInstallerExperiment stops the installer experiment
func StopInstallerExperiment(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "stop_installer_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to stop installer experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	err = msiexec("stable", datadogInstaller, "/i", nil)
	return err
}

// PromoteInstallerExperiment promotes the installer experiment
func PromoteInstallerExperiment(_ context.Context) error {
	return nil
}
