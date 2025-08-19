// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package ssistatusimpl

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/ssi"
)

// autoInstrumentationStatus checks if the APM auto-instrumentation is enabled on the host. This will return false on Kubernetes
func (c *ssiStatusComponent) autoInstrumentationStatus() (bool, []string, error) {
	// Ideally we would call the installer package to check if the injector is installed (using the status), but doing so
	// significantly increases the memory footprint of the agent, so we check the filesystem directly.

	// TODO: fix this
	_, err := os.Stat("/opt/datadog-packages/datadog-apm-inject")
	injectorInstalled := false
	if err == nil {
		injectorInstalled = true
	} else if !os.IsNotExist(err) {
		return false, nil, fmt.Errorf("could not check if injector package is installed: %w", err)
	}

	instrumentationStatus, err := ssi.GetInstrumentationStatus()
	if err != nil {
		return false, nil, fmt.Errorf("could not get APM injection status: %w", err)
	}

	instrumentationModes := []string{}
	if instrumentationStatus.HostInstrumented {
		instrumentationModes = append(instrumentationModes, "host")
	}
	if instrumentationStatus.DockerInstalled && instrumentationStatus.DockerInstrumented {
		instrumentationModes = append(instrumentationModes, "docker")
	}

	return injectorInstalled && (instrumentationStatus.HostInstrumented || (instrumentationStatus.DockerInstrumented && instrumentationStatus.DockerInstalled)), instrumentationModes, nil
}
