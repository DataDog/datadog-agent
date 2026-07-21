// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !windows

package coat

import "context"

func detectLegacySupervisor(_ context.Context, _ MigratableService) ManagementMode {
	return ManagementModeNone
}
