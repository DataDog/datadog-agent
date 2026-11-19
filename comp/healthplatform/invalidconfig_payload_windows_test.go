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
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestInvalidConfigPayloadIgnoresHostFleetPolicy checks that the scenarios are isolated from a Fleet Automation
// policy configured on the host. On Windows, both config loaders fall back to the fleet_policies_dir registry value
// when the setting is unset, so a host policy would be merged into the scenario configurations:
//   - the datadog.yaml policy sends the health reports to an unreachable intake: the scenario only passes if it's ignored
//   - the system-probe.yaml policy sets a setting the reports don't show: it must not reach the loaded configuration
func TestInvalidConfigPayloadIgnoresHostFleetPolicy(t *testing.T) {
	requireSchema(t)
	initProcessGlobals()

	policyDir := t.TempDir()
	datadogPolicy := "health_platform:\n  forwarder:\n    dd_url: \"http://127.0.0.1:1\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "datadog.yaml"), []byte(datadogPolicy), 0o600))
	const sysprobePolicyLogFile = "host-fleet-policy-system-probe.log"
	sysprobePolicy := "system_probe_config:\n  log_file: " + sysprobePolicyLogFile + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "system-probe.yaml"), []byte(sysprobePolicy), 0o600))
	configmock.SetFleetPoliciesDirFromRegistry(t, policyDir)

	synctest.Test(t, func(t *testing.T) {
		sysprobeConfig := runInvalidConfigScenario(t, "file_types")
		assert.NotEqual(t, sysprobePolicyLogFile, sysprobeConfig.GetString("system_probe_config.log_file"),
			"the host system-probe fleet policy must not be merged")
	})
}
