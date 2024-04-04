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
	// Patches is the number of patch requests sent by the patcher to the kubernetes api server
	Patches = telemetry.NewCounterWithOpts(
		subsystem,
		"patches",
		[]string{"owner_kind", "owner_name", "namespace", "status"},
		"Tracks the number of patch requests sent by the patcher to the kubernetes api server",
		commonOpts,
	)
)
