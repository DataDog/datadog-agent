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

// Configuration keys for the private action runner.
// Duplicated from pkg/privateactionrunner/config.go because comp/
// packages cannot import pkg/ packages (depguard rule).
const (
	Enabled                = "private_action_runner.enabled"
	SelfEnroll             = "private_action_runner.self_enroll"
	APIKeyOnlyEnrollment   = "private_action_runner.api_key_only_enrollment"
	SkipConnectionCreation = "private_action_runner.skip_connection_creation"
	PrivateKey             = "private_action_runner.private_key"
	URN                    = "private_action_runner.urn"
	ActionsAllowlist       = "private_action_runner.actions_allowlist"
	DefaultActionsEnabled  = "private_action_runner.default_actions_enabled"
	IdleTimeoutSeconds     = "private_action_runner.idle_timeout_seconds"

	ExecutorSocketPath = "private_action_runner.executor.socket_path"
	SplitEnabled       = "private_action_runner.split_enabled"
)
