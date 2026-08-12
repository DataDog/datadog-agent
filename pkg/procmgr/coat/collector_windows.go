// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package coat

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func detectLegacySupervisor(_ context.Context, service MigratableService) ManagementMode {
	if service.LegacyWindowsService == "" {
		return ManagementModeNone
	}
	running, err := winutil.IsServiceRunning(service.LegacyWindowsService)
	if err == nil && running {
		return ManagementModeWindowsService
	}
	return ManagementModeNone
}
