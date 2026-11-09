// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package privateactionrunner provides a component that enables private actions executions
package privateactionrunner

import "errors"

// team: action-platform

// Component is the component type.
type Component interface {
}

// ErrNotEnabled is returned when the private action runner is not enabled
var ErrNotEnabled = errors.New("private action runner is not enabled")

// ErrSplitDeployment is returned when the private action runner runs in split
// deployment mode, where par-control owns OPMS polling.
var ErrSplitDeployment = errors.New("private action runner is running in split deployment mode")
