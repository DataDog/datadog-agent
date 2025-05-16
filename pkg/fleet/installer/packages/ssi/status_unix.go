// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package ssi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	iexec "github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
)

// GetInstrumentationStatus contains the status of the APM auto-instrumentation.
func GetInstrumentationStatus() (status APMInstrumentationStatus, err error) {
	// Host is instrumented if the ld.so.preload file contains the apm injector
	ldPreloadContent, err := os.ReadFile("/etc/ld.so.preload")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, fmt.Errorf("could not read /etc/ld.so.preload: %w", err)
	}
	if bytes.Contains(ldPreloadContent, []byte("/opt/datadog-packages/datadog-apm-inject/stable/inject")) {
		status.HostInstrumented = true
	}

	// Docker is installed if the docker binary is in the PATH
	_, err = exec.LookPath("docker")
	if err != nil && errors.Is(err, exec.ErrNotFound) {
		return status, nil
	} else if err != nil {
		return status, fmt.Errorf("could not check if docker is installed: %w", err)
	}
	status.DockerInstalled = true

	// Docker is instrumented if there is the injector runtime in its configuration
	// We're not retrieving the default runtime from the docker daemon as we are not
	// root
	dockerConfigContent, err := os.ReadFile("/etc/docker/daemon.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, fmt.Errorf("could not read /etc/docker/daemon.json: %w", err)
	} else if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if bytes.Contains(dockerConfigContent, []byte("/opt/datadog-packages/datadog-apm-inject/stable/inject")) {
		status.DockerInstrumented = true
	}

	return status, nil
}

// isInjectorPkgInstalled checks if the APM injector package is installed on the host.
func isInjectorPkgInstalled(ctx context.Context) (bool, error) {
	installerPath, err := exec.LookPath("datadog-installer")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, nil // Installer is not installed; thus no SSI
		}
		return false, fmt.Errorf("could not check if datadog-installer is installed: %w", err)
	}
	installerExec := iexec.NewInstallerExec(&env.Env{}, installerPath)
	return installerExec.IsInstalled(ctx, "datadog-apm-inject")
}

// IsAutoInstrumentationEnabled checks if the APM auto-instrumentation is enabled on the host. This is scoped to Linux hosts and will return false in Kubernetes.
func IsAutoInstrumentationEnabled(ctx context.Context) (bool, error) {
	injectorInstalled, err := isInjectorPkgInstalled(ctx)
	if err != nil {
		return false, fmt.Errorf("could not check if injector package is installed: %w", err)
	}
	instrumentationStatus, err := GetInstrumentationStatus()
	if err != nil {
		return false, fmt.Errorf("could not get APM injection status: %w", err)
	}
	return injectorInstalled && (instrumentationStatus.HostInstrumented || (instrumentationStatus.DockerInstrumented && instrumentationStatus.DockerInstalled)), nil
}
