// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package ssi

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
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
func isInjectorPkgInstalled() (bool, error) {
	packagesDB, err := db.New(filepath.Join(paths.PackagesPath, "packages.db"), db.WithTimeout(10*time.Second), db.WithReadOnly(true))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil // Database does not exist, so installer is not installed; thus no SSI
		}
		return false, fmt.Errorf("could not access datadog packages db: %w", err)
	}
	defer packagesDB.Close()

	hasPackage, err := packagesDB.HasPackage("datadog-apm-inject")
	if err != nil {
		return false, fmt.Errorf("could not list packages: %w", err)
	}
	return hasPackage, nil
}

// IsAutoInstrumentationEnabled checks if the APM auto-instrumentation is enabled on the host. This is scoped to Linux hosts and will return false in Kubernetes.
func IsAutoInstrumentationEnabled() (bool, error) {
	injectorInstalled, err := isInjectorPkgInstalled()
	if err != nil {
		return false, fmt.Errorf("could not check if injector package is installed: %w", err)
	}
	instrumentationStatus, err := GetInstrumentationStatus()
	if err != nil {
		return false, fmt.Errorf("could not get APM injection status: %w", err)
	}
	return injectorInstalled && (instrumentationStatus.HostInstrumented || (instrumentationStatus.DockerInstrumented && instrumentationStatus.DockerInstalled)), nil
}
