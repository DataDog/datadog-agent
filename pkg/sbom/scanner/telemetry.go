// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package scanner

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	subsystem        = "trivy"
	sourceContainerd = "containerd"
)

var (
	// sbomAttempts tracks sbom collection attempts.
	sbomAttempts = telemetry.NewCounterWithOpts(
		subsystem,
		"sbom_attempts",
		[]string{"source", "type"},
		"Number of sbom failures by (source, type)",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	// sbomFailures tracks sbom collection attempts that fail.
	sbomFailures = telemetry.NewCounterWithOpts(
		subsystem,
		"sbom_errors",
		[]string{"source", "type", "reason"},
		"Number of sbom failures by (source, type, reason)",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)
