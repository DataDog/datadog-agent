// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installers defines the shared types for the Pulumi-free installer layer.
package installers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// Context is an alias for common.Context that installer packages use as their
// first parameter. It is satisfied by:
//   - *testing.T via FromT (in test code)
//   - BaseSuite[Env] directly (since BaseSuite now implements common.Context)
//   - CLIContext (in the e2e-install CLI, which calls os.Exit(1) on FailNow)
//
// Since common.Context no longer contains T() *testing.T, installer functions
// can be called from non-test programs without any test framework bootstrapping.
type Context = common.Context

// FromT wraps *testing.T to satisfy Context. Use this when you have a
// *testing.T (e.g. in a custom SetupSuite) and need to pass it to an installer.
func FromT(t *testing.T) Context {
	return &testingTContext{t: t}
}

// testingTContext wraps *testing.T to implement common.Context.
type testingTContext struct{ t *testing.T }

func (c *testingTContext) Errorf(format string, args ...any) { c.t.Errorf(format, args...) }
func (c *testingTContext) FailNow(format string, args ...any) {
	c.t.Helper()
	c.t.Logf(format, args...)
	c.t.FailNow()
}
func (c *testingTContext) Logf(format string, args ...any) { c.t.Logf(format, args...) }
func (c *testingTContext) Helper()                         { c.t.Helper() }
func (c *testingTContext) Cleanup(fn func())               { c.t.Cleanup(fn) }
func (c *testingTContext) SessionOutputDir() string        { return "" }
