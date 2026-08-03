// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"path/filepath"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// FleetConfigOverride sets fleet_policies_dir for every Windows agent binary that loads
// config through pkg/config/setup (registered in fixup_init; system-probe calls it directly).
//
// Resolution order (first non-empty wins): datadog.yaml / DD_FLEET_POLICIES_DIR, then the
// registry experiment path, then defaultStableFleetPoliciesDir. Mirrors dd-procmgr config
// gates in pkg/procmgr/rust/src/config_gate.rs.
//
// The stable ProgramData fallback is intentional global parity with procmgr-managed children
// (process-agent, PAR, DDOT): they no longer carry DD_FLEET_POLICIES_DIR in processes.d, so
// all Windows binaries must resolve the same managed policy directory without per-service env.
//
// Standalone installs are unaffected: comp/core/config and system-probe call MergeFleetPolicy
// only after fleet_policies_dir is set, and MergeFleetPolicy no-ops when the policy YAML file
// is absent (pkg/config/nodetreemodel/config.go). On fleet-managed hosts between experiments
// (registry empty, stable managed datadog.yaml present), the stable layer now merges as
// SourceFleetPolicies — previously nothing merged in that window.
//
// Linux sets fleet_policies_dir via environment in experiment systemd units; after the viper
// migration we may replace this override with a Windows registry config source.
func FleetConfigOverride(config pkgconfigmodel.Config) {
	// Prioritize the value set in the config file / env var
	if config.IsConfigured("fleet_policies_dir") {
		return
	}

	val := winutil.ReadFleetPoliciesDirFromRegistry()
	if val == "" {
		val = defaultStableFleetPoliciesDir()
	}
	if val == "" {
		return
	}

	config.Set("fleet_policies_dir", val, pkgconfigmodel.SourceAgentRuntime)
}

// defaultStableFleetPoliciesDir returns the stable managed fleet policies directory under
// ProgramData. Matches pkg/fleet/installer/paths.FleetPoliciesDirForManagedProcess without
// importing fleet/installer (circular dependency with config/setup).
//
// Used as the global FleetConfigOverride fallback, not only for dd-procmgr: every Windows
// binary that merges fleet policy YAML must find the same stable directory when the registry
// experiment path is unset.
func defaultStableFleetPoliciesDir() string {
	dataDir, err := winutil.GetProgramDataDirForProduct("Datadog Agent")
	if err != nil || dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "Installer", "managed", "datadog-agent", "stable")
}
