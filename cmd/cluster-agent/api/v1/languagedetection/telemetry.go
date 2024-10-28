// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const subsystem = "language_detection_dca_handler"

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

var (
	// ProcessedRequests tracks the number requests processed by the handler
	ProcessedRequests = telemetry.NewCounterWithOpts(
		subsystem,
		"processed_requests",
		[]string{"status"},
		"Tracks the number of requests processed by the handler",
		commonOpts,
	)
)
