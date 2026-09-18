// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"testing"
	_ "unsafe" // for go:linkname

	_ "github.com/DataDog/dd-trace-go/v2/civisibility" // links in the CI Visibility implementation
)

// instrumentTestifySuiteRun is the same hook Orchestrion's automatic
// instrumentation weaves into testify/suite.Run itself (aspect
// `testify.suite.Run` in dd-trace-go's
// `internal/civisibility/integrations/gotesting/orchestrion.yml`), called
// here by hand instead, immediately before the real, unmodified
// `suite.Run` call in this file's `Run` function. Without it, CI
// Visibility spans still fire for every subtest (via the
// bazel/rules/go_civisibility stdlib patch), but are misattributed to
// testify's own source file instead of the actual suite.
//
//go:linkname instrumentTestifySuiteRun github.com/DataDog/dd-trace-go/v2/internal/civisibility/integrations/gotesting.instrumentTestifySuiteRun
func instrumentTestifySuiteRun(t *testing.T, s any)
