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

// FleetConfigOverride sets fleet_policies_dir from the Windows registry when present,
// otherwise from the stable managed fleet policies directory under ProgramData.
//
// This value tells the agent to load fleet policy YAML (experiments or stable config).
// Linux sets this option with an environment variable in the experiment's systemd unit file,
// so we need a different approach for Windows. After the viper migration is complete, we can
// consider replacing this override with a Windows Registry config source.
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

// defaultStableFleetPoliciesDir matches pkg/fleet/installer/paths.FleetPoliciesDirForManagedProcess
// stable fallback without importing fleet/installer (circular dependency with config/setup).
func defaultStableFleetPoliciesDir() string {
	dataDir, err := winutil.GetProgramDataDirForProduct("Datadog Agent")
	if err != nil || dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "Installer", "managed", "datadog-agent", "stable")
}
