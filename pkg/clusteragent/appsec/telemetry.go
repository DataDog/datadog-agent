// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// watchedChangesCounter tracks the number of changes detected by the appsec injector for watched resources
	// Tags: proxy_type, operation, success
	watchedChangesCounter = telemetry.NewCounterWithOpts(
		"appsec_injector",
		"watched_changes",
		[]string{"proxy_type", "operation", "success"},
		"Tracks the number of changes detected by the appsec injector for the watched resources",
		telemetry.DefaultOptions,
	)
)
