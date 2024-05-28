// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const subsystem = "autoscaling_webhook"

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

var injections = telemetry.NewGaugeWithOpts(
	subsystem,
	"resource_injection",
	[]string{
		"resource", // CPU / Memory
		"namespace",
		"owner",     // Kube deployment / statefulset
		"container", // Container name
		"type",      // Request / Limit
		"rec_id",    // Hash of the recommendations
	},
	"The injections performed on a specific workload",
	commonOpts,
)
