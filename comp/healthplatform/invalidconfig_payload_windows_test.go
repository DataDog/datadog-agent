// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && windows

package healthplatform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestInvalidConfigPayloadIgnoresHostFleetPolicy checks that the scenarios are isolated from a Fleet Automation
// policy configured on the host. On Windows, the config loader falls back to the fleet_policies_dir registry value
// when the setting is unset, so a host policy would be merged into the scenario configuration. The policy here sends
// the health reports to an unreachable intake: the scenario only passes if the policy is ignored.
func TestInvalidConfigPayloadIgnoresHostFleetPolicy(t *testing.T) {
	requireSchema(t)

	policyDir := t.TempDir()
	policy := "health_platform:\n  forwarder:\n    dd_url: \"http://127.0.0.1:1\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "datadog.yaml"), []byte(policy), 0o600))
	configmock.SetFleetPoliciesDirFromRegistry(t, policyDir)

	runInvalidConfigScenario(t, "file_types")
}
