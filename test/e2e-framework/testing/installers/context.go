// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installers defines the shared types for the Pulumi-free installer layer.
package installers

import "testing"

// Context is the execution context an installer runs in. It is satisfied by:
//   - a test, via a thin adapter wrapping *testing.T (used by PostProvision)
//   - the e2e-install CLI, which builds a real *testing.T via go test -c
//
// It is intentionally NOT named after "testing" and does NOT expose T() *testing.T
// so that callers are not forced into a test context at the type level. However,
// since the current client infrastructure (client.Host.MustExecute etc.) is still
// coupled to *testing.T via common.Context.T(), the CLI path uses go test -c to
// obtain a real *testing.T transparently. This coupling will be removed in Phase 4c.
//
// Within installers, use this as the first parameter instead of *testing.T.
type Context interface {
	// testify require.TestingT methods — makes Context a drop-in for
	// require.NoError(ctx, ...) calls inside installer functions.
	Errorf(format string, args ...any)
	FailNow()

	Logf(format string, args ...any)
	Helper()
	Cleanup(fn func())
}

// FromT wraps *testing.T to satisfy Context. Use this in test code where you
// have a *testing.T and need to pass it to an installer function expecting Context.
func FromT(t *testing.T) Context {
	return t
}
