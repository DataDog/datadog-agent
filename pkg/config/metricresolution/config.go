// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package metricresolution exposes the single environment toggle used by the
// throwaway global one-second metric-resolution experiment.
package metricresolution

import (
	"os"
	"strconv"
)

// EnabledEnvVar selects the hardcoded one-second treatment behavior.
const EnabledEnvVar = "DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED"

// Enabled reports whether the throwaway treatment is enabled. Invalid or unset
// values are disabled so normal Agent behavior remains the default.
func Enabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv(EnabledEnvVar))
	return err == nil && enabled
}
