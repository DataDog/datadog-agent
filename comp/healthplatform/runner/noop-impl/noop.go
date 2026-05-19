// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noopimpl provides a no-op implementation of the health platform runner.
// Used when the health platform is disabled via configuration.
package noopimpl

import (
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

type noopRunner struct{}

// NewNoopComponent creates a no-op runner (health platform disabled).
func NewNoopComponent() runnerdef.Component {
	return &noopRunner{}
}

func (n *noopRunner) Run(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
	return nil, nil
}
