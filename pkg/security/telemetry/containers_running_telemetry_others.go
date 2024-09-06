// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package telemetry

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// ContainersRunningTelemetry reports environment information (e.g containers running) when the runtime security component is running
type ContainersRunningTelemetry struct{}

// NewContainersRunningTelemetry creates a new ContainersRunningTelemetry instance (not supported on non-linux platforms)
func NewContainersRunningTelemetry(_ *config.RuntimeSecurityConfig, _ statsd.ClientInterface, _ workloadmeta.Component) (*ContainersRunningTelemetry, error) {
	return nil, nil
}

// Run starts the telemetry collection
func (t *ContainersRunningTelemetry) Run(_ context.Context) {}
