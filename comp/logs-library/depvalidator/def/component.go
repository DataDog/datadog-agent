// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package depvalidator provides validation for optional dependencies in the logs library.
package depvalidator

// team: agent-log-pipelines

// Component provides logs-enabled status and dependency validation.
type Component interface {
	// LogsEnabled returns true if the logs agent is enabled via configuration.
	LogsEnabled() bool

	// ValidateDependencies checks that all option.Option[T] fields in the given
	// struct have values set (i.e., are not None). Returns an error on the first
	// field that is missing a value, or nil if all fields are valid.
	//
	// The deps parameter should be a struct (or pointer to struct) containing
	// option.Option[T] fields, typically a component's Requires struct.
	//
	// To skip validation for specific fields, use the struct tag `depvalidator:"optional"`:
	//
	//	type Requires struct {
	//	    Required option.Option[foo.Component]                          // validated
	//	    Optional option.Option[bar.Component] `depvalidator:"optional"` // skipped
	//	}
	ValidateDependencies(deps any) error

	// ValidateIfEnabled checks if logs are enabled and validates dependencies.
	// Returns:
	//   - nil if logs are enabled and all dependencies are valid
	//   - ErrLogsDisabled if logs are disabled
	//   - a validation error if logs are enabled but a dependency is missing
	ValidateIfEnabled(deps any) error
}

// ErrLogsDisabled is returned by ValidateIfEnabled when logs are disabled.
var ErrLogsDisabled = errLogsDisabled{}

type errLogsDisabled struct{}

func (errLogsDisabled) Error() string { return "logs are disabled" }
