// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kubehealth provides a dependency-injectible health object for kubernetes liveness checks
package kubehealth

import "github.com/DataDog/datadog-agent/pkg/status/health"

// team: agent-log-pipelines

// Component is a wrapper around the health package to allow for easier registration of health checks
type Component interface {
	RegisterReadiness(name string, options ...health.Option) *health.Handle
	RegisterLiveness(name string, options ...health.Option) *health.Handle
	RegisterStartup(name string, options ...health.Option) *health.Handle
	Deregister(handle *health.Handle) error
}
