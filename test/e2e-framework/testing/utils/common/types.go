// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

// Context defines the execution context for e2e test operations.
// It is satisfied by *testing.T (in tests) and by CLI context implementations
// (outside tests). Removing *testing.T from this interface is what allows the
// installer layer to run in non-test programs.
//
// The Errorf + FailNow pair satisfies testify's require.TestingT, so callers
// can pass a Context directly to require.NoError(ctx, err) etc.
type Context interface {
	// Errorf reports a non-fatal test/operation failure.
	Errorf(format string, args ...any)
	// FailNow marks the current operation as failed and stops execution.
	FailNow()
	// Logf logs a message.
	Logf(format string, args ...any)
	// Helper marks the calling function as a helper (no-op outside tests).
	Helper()
	// Cleanup registers a function to be called when the operation finishes.
	Cleanup(fn func())
	// SessionOutputDir returns the directory for output files and artifacts.
	SessionOutputDir() string
}

// Initializable defines the interface for an object that needs to be initialized
type Initializable interface {
	Init(Context) error
}

// Diagnosable defines the interface for an object that can dump diagnostic information
// and store files in an output directory
type Diagnosable interface {
	Diagnose(outputDir string) (string, error)
}

// Coverageable defines the interface for an environment that can generage coverage information about the agent under test
// and store files in an output directory
type Coverageable interface {
	Coverage(outputDir string) (string, error)
}

// CoverageRequiredOverrideable defines an optional interface for environments that support overriding
// the default coverage required setting per agent. Each key in the map must match a CoverageTargetSpec.AgentName.
type CoverageRequiredOverrideable interface {
	SetCoverageRequiredOverride(overrides map[string]bool)
}
