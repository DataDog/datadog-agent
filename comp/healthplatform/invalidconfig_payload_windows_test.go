// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && windows

package healthplatform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// agentRegistryKey is the key the Agent reads fleet_policies_dir from when the setting isn't configured
	agentRegistryKey = `SOFTWARE\Datadog\Datadog Agent`
	// fleetPoliciesDirValue is the registry value holding the Fleet Automation policies directory
	fleetPoliciesDirValue = "fleet_policies_dir"
)

// TestInvalidConfigPayloadIgnoresHostFleetPolicy checks that the scenarios are isolated from a Fleet Automation
// policy configured on the host. On Windows, the config loader falls back to the registry when fleet_policies_dir
// is unset, so a host policy would be merged into the scenario configuration. The policy here sends the health
// reports to an unreachable intake: the scenario only passes if the policy is ignored.
func TestInvalidConfigPayloadIgnoresHostFleetPolicy(t *testing.T) {
	requireSchema(t)

	policyDir := t.TempDir()
	policy := "health_platform:\n  forwarder:\n    dd_url: \"http://127.0.0.1:1\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "datadog.yaml"), []byte(policy), 0o600))
	setHostFleetPoliciesDir(t, policyDir)
	require.Equal(t, policyDir, winutil.ReadFleetPoliciesDirFromRegistry(), "the Agent must see the host fleet policy")

	runInvalidConfigScenario(t, "file_types")
}

// setHostFleetPoliciesDir sets the fleet_policies_dir registry value read by the Agent, and restores the previous
// value at the end of the test. The test is skipped when the registry key can't be written (not an administrator).
func setHostFleetPoliciesDir(t *testing.T, dir string) {
	t.Helper()
	key, existed, err := registry.CreateKey(registry.LOCAL_MACHINE, agentRegistryKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Skipf(`cannot write HKLM\%s (%v): run the test as an administrator`, agentRegistryKey, err)
	}
	require.NoError(t, err)
	defer key.Close()

	previous, _, err := key.GetStringValue(fleetPoliciesDirValue)
	hadPrevious := err == nil
	if !hadPrevious {
		require.ErrorIs(t, err, registry.ErrNotExist, "unexpected %s registry value", fleetPoliciesDirValue)
	}

	t.Cleanup(func() {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, agentRegistryKey, registry.SET_VALUE)
		if !assert.NoError(t, err, "cannot restore the %s registry value", fleetPoliciesDirValue) {
			return
		}
		if hadPrevious {
			assert.NoError(t, key.SetStringValue(fleetPoliciesDirValue, previous))
		} else {
			assert.NoError(t, key.DeleteValue(fleetPoliciesDirValue))
		}
		key.Close()
		if !existed {
			// the key was created for the test: remove it, unless something else was added to it meanwhile
			_ = registry.DeleteKey(registry.LOCAL_MACHINE, agentRegistryKey)
		}
	})
	require.NoError(t, key.SetStringValue(fleetPoliciesDirValue, dir))
}
