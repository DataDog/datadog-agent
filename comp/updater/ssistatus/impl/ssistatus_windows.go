// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package ssistatusimpl

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/ssi"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// autoInstrumentationStatus checks if the APM auto-instrumentation is enabled on the host.
func (c *ssiStatusComponent) autoInstrumentationStatus() (bool, []string, error) {
	injectorInstalled := false
	_, err := os.Stat(filepath.Join(paths.PackagesPath, "datadog-apm-inject"))
	if err == nil {
		injectorInstalled = true
	} else if !os.IsNotExist(err) {
		return false, nil, fmt.Errorf("could not check if injector package is installed: %w", err)
	}

	instrumentationStatus, err := ssi.GetInstrumentationStatus()
	if err != nil {
		return false, nil, fmt.Errorf("could not get APM injection status: %w", err)
	}

	var modes []string
	if instrumentationStatus.IISInstrumented {
		modes = append(modes, "iis")
	}
	if instrumentationStatus.HostInstrumented {
		modes = append(modes, "host")
	}

	return injectorInstalled && len(modes) > 0, modes, nil
}
