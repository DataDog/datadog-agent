// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package privateactionrunner contains e2e tests for the Private Action Runner rshell bundle.
package privateactionrunner

import (
	"testing"
)

// generateTestRunnerIdentity is a test-local alias for GenerateTestRunnerIdentity.
func generateTestRunnerIdentity(t *testing.T) (urn string, privateKeyB64 string) {
	return GenerateTestRunnerIdentity(t)
}
