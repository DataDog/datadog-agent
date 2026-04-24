// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package ssistatusimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/ssi"
)

// autoInstrumentationStatus checks if the APM auto-instrumentation is enabled on the host.
func (c *ssiStatusComponent) autoInstrumentationStatus() (bool, []string, error) {
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

	return len(modes) > 0, modes, nil
}
