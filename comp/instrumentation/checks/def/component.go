// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package checks defines the DatadogInstrumentation checks handler component.
package checks

import "github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"

// team: container-platform

// Component validates and reconciles the checks section of a DatadogInstrumentation.
// Its handler methods may be called concurrently by the controller and admission webhook.
type Component interface {
	instrumentation.Handler

	// SetServiceTargetEnabled must be called during startup, before the controller
	// and admission webhook start using the handler.
	SetServiceTargetEnabled(enabled bool)
}
