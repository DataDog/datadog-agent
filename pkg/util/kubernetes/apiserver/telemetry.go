// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const subsystem = "apiserver"

var (
	// apiServerTimeouts tracks timeouts to kubernetes apiserver done by the Agent.
	clientTimeouts = telemetry.NewCounterWithOpts(
		subsystem,
		"client_timeouts",
		[]string{},
		"Count of requests to the apiserver which have timed out. Consider increasing the kubernetes_apiserver_client_timeout setting.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	// kube_cache_sync_timeouts tracks timeouts to kubernetes apiserver done by the Agent.
	cacheSyncTimeouts = telemetry.NewCounterWithOpts(
		subsystem,
		"cache_sync_timeouts",
		[]string{},
		"Count of kubernetes cache requests which have timed out. Consider increasing the kube_cache_sync_timeout_seconds setting.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)
