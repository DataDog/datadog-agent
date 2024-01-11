// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !kubeapiserver || !orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import "github.com/DataDog/datadog-agent/pkg/collector/check"

const (
	// Enabled is true if the check is enabled
	Enabled = false
	// CheckName is the name of the check
	CheckName = "orchestrator"
)

// New creates a new check instance
func New() check.Check {
	return nil
}
