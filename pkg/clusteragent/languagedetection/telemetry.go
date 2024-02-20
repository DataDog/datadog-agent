// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const subsystem = "language_detection_patcher"

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

var (
	// PatchRetries determines the number of times a patch request fails and is retried for a gived
	PatchRetries = telemetry.NewCounterWithOpts(
		subsystem,
		"retries",
		[]string{"owner_kind", "owner_name", "namespace"},
		"Tracks the number of retries while patching deployments with language annotations",
		commonOpts,
	)

	// SuccessPatches tracks the number of successful annotation patch operations
	SuccessPatches = telemetry.NewCounterWithOpts(
		subsystem,
		"success_patch",
		[]string{"owner_kind", "owner_name", "namespace"},
		"Tracks the number of successful annotation patch operations",
		commonOpts,
	)

	// FailedPatches tracks the number of failing annotation patch operations
	FailedPatches = telemetry.NewCounterWithOpts(
		subsystem,
		"fail_patch",
		[]string{"owner_kind", "owner_name", "namespace"},
		"Tracks the number of failing annotation patch operations",
		commonOpts,
	)

	// SkippedPatches tracks the number of times a patch was skipped because no new languages are detected
	SkippedPatches = telemetry.NewCounterWithOpts(
		subsystem,
		"skipped_patch",
		[]string{"owner_kind", "owner_name", "namespace"},
		"Tracks the number of times a patch was skipped because no new languages are detected or old languages were expired",
		commonOpts,
	)
)
