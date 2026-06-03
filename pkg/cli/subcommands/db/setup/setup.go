// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup defines shared types for the DBM setup CLI logic layer.
package setup

// Flavor is the detected Postgres platform type.
type Flavor string

const (
	FlavorSelfHosted Flavor = "self_hosted"
	FlavorRDS        Flavor = "rds"
	FlavorAurora     Flavor = "aurora"
	FlavorCloudSQL   Flavor = "cloud_sql"
)

// OperationKind classifies what an Operation does so the output layer can
// render it without knowing anything about the DBMS.
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

// Operation is a single step in the setup plan.
type Operation struct {
	Kind        OperationKind
	Description string
	// SQL is the statement to execute; empty for MANUAL_STEP and SKIP.
	SQL string
	// Args are the bound parameters for parameterized SQL.
	Args []any
	// RedactSQL suppresses SQL/args in output (used for credential operations).
	RedactSQL bool
	// Database is set for per-database operations; empty for cluster-level ops.
	Database string
	// ManualInstruction is the human-readable guidance for MANUAL_STEP ops.
	ManualInstruction string
	// SettingName is populated for server-setting operations.
	SettingName string
	// RequiresRestart is true for postmaster-context settings.
	RequiresRestart bool

	// Status and Error are populated by Apply.
	Status OperationStatus
	Error  error
}

// SetupResult is the complete outcome of an Apply run.
type SetupResult struct {
	Operations   []*Operation
	Flavor       Flavor
	PGVersion    int
	RestartNeeded bool
	ManualSteps  bool
	// Outcome is "success", "failure", or "dry_run".
	Outcome string
}

// DBMSetup is the interface implemented by each DBMS setup (postgres, mysql, …).
type DBMSetup interface {
	Detect() (*SetupState, error)
	Plan(state *SetupState, cfg Config) ([]*Operation, error)
	Apply(ops []*Operation) *SetupResult
}

// Config holds all user-provided flags for a setup run.
type Config struct {
	DatadogUser     string
	DatadogPassword string
	Databases       []string
	AllDatabases    bool
	DryRun          bool
	Output          string
}

// SetupState is the read-only snapshot produced by Detect.
type SetupState struct {
	Flavor          Flavor
	PGVersion       int
	UserExists      bool
	CurrentSettings map[string]string
	// PendingRestart lists setting names that have pending_restart=true.
	PendingRestart []string
	Databases      []string
}
