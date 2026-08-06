// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup defines shared types for the DBM integration setup CLI.
package setup

// Flavor is the detected Postgres platform type.
type Flavor string

const (
	FlavorSelfHosted Flavor = "self_hosted"
	FlavorRDS        Flavor = "rds"
	FlavorAurora     Flavor = "aurora"
	FlavorCloudSQL   Flavor = "cloud_sql"
	FlavorAzure      Flavor = "azure"
)

// OperationKind classifies what an Operation does.
type OperationKind string

const (
	KindSQL        OperationKind = "SQL"
	KindAlterSys   OperationKind = "ALTER_SYSTEM"
	KindReload     OperationKind = "ALTER_SYSTEM_RELOAD"
	KindManualStep OperationKind = "MANUAL_STEP"
	KindSkip       OperationKind = "SKIP"
)

// OperationStatus is the result of executing an Operation during Apply.
type OperationStatus string

const (
	StatusCompleted OperationStatus = "completed"
	StatusSkipped   OperationStatus = "skipped"
	StatusFailed    OperationStatus = "failed"
	StatusPending   OperationStatus = "pending"
	StatusManual    OperationStatus = "manual"
)

// Operation is a single step in the setup plan, as returned by the Python runner.
type Operation struct {
	Kind              OperationKind   `json:"kind"`
	Description       string          `json:"description"`
	Database          string          `json:"database,omitempty"`
	SettingName       string          `json:"setting_name,omitempty"`
	RequiresRestart   bool            `json:"requires_restart,omitempty"`
	ManualInstruction string          `json:"manual_instruction,omitempty"`
	Status            OperationStatus `json:"status,omitempty"`
	Error             string          `json:"error,omitempty"`
}

// SetupResult is the complete outcome of a setup run, as returned by the Python runner.
type SetupResult struct {
	Operations             []*Operation             `json:"operations"`
	Flavor                 Flavor                   `json:"flavor"`
	PGVersion              int                      `json:"pg_version"`
	RestartNeeded          bool                     `json:"restart_needed"`
	ManualSteps            bool                     `json:"manual_steps"`
	Outcome                string                   `json:"outcome"`
	OptionalRestartPending []OptionalRestartSetting `json:"optional_restart_pending,omitempty"`
}

// OptionalRestartSetting describes a setting that was deferred because it
// is optional and requires a restart to apply.
type OptionalRestartSetting struct {
	Name        string `json:"name"`
	Desired     string `json:"desired"`
	Current     string `json:"current"`
	Description string `json:"description"`
}

// PythonResult is the top-level JSON envelope written by the integration's
// datadog_checks.postgres.setup module.
type PythonResult struct {
	Success bool         `json:"success"`
	Error   string       `json:"error,omitempty"`
	Result  *SetupResult `json:"result,omitempty"`
}
