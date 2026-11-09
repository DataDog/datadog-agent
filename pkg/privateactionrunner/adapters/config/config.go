// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"time"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// Config exposes the settings PAR consumers may access. It deliberately excludes
// generic key lookup, mutation, and access to the underlying Agent component.
// Methods read current values; implementations must not cache configuration.
type Config interface {
	AgentAPIURL() (string, error)
	HasAPIKey() bool
	LogProcessingRules() ([]*logsconfig.ProcessingRule, error)
	CaptureSocketPaths() (capture string, systemProbe string)
	RunnerID() (string, error)
	HTTPTimeout() time.Duration
	HTTPAllowlist() []string
	HTTPAllowIMDSEndpoint() bool
	KubernetesAllowedCustomResources() (resources []string, configured bool)
	RestrictedShellAllowedPaths() (paths []string, configured bool)
	RestrictedShellAllowedCommands() (commands []string, configured bool)
	RestrictedShellAllowedSystemServices() map[string][]string
	RestrictedShellElevatableCommands() []string
	RestrictedShellDisableDetailedTelemetry() bool
	RestrictedShellPrivilegedEnabled() bool
	RestrictedShellPrivilegedSocket() string
	TaskTimeoutSeconds() int32
	TaskConcurrency() int32
	IsActionAllowed(bundle, action string) bool
	APIBaseURL() (string, error)
	OPMSExtraHeaders() map[string]string
}
