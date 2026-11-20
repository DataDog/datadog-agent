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
	// watchedChangesCounter counts the work-item attempts of the appsec injector, retries included
	watchedChangesCounter = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"appsec_injector",
		"watched_changes",
		[]string{"proxy_type", "operation", "success", "reason"},
		"Counts the appsec injector work-item attempts (retries included) for the watched resources",
		telemetry.DefaultOptions,
	)
)
