// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

var (
	// watchedChangesCounter tracks the number of changes detected by the appsec injector for watched resources
	// Tags: proxy_type, operation, success
	watchedChangesCounter = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"appsec_injector",
		"watched_changes",
		[]string{"proxy_type", "operation", "success"},
		"Tracks the number of changes detected by the appsec injector for the watched resources",
		telemetry.DefaultOptions,
	)

	// enabledGauge shows where the injector runs, which the counters only show once it processes a change
	enabledGauge = telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
		"appsec_injector",
		"enabled",
		[]string{"proxy_type", "mode"},
		"Set to 1 for each proxy type the appsec injector runs for",
		telemetry.DefaultOptions,
	)
)
