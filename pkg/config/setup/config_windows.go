// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"golang.org/x/sys/windows/registry"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	// defaultGuiPort is the default GUI port on Windows
	defaultGuiPort = 5002
)

func osinit() {
	// Fleet Automation
	pkgconfigmodel.AddOverrideFunc(FleetConfigOverride)
}

// FleetConfigOverride sets the fleet_policies_dir config value to the value set in the registry.
//
// This value tells the agent to load a config experiment from Fleet Automation.
//
// Linux sets this option with an environment variable in the experiment's systemd unit file,
// so we need a different approach for Windows. After the viper migration is complete, we can
// consider replacing this override with a Windows Registry config source.
func FleetConfigOverride(config pkgconfigmodel.Config) {
	// Prioritize the value set in the config file / env var
	if config.IsConfigured("fleet_policies_dir") {
		return
	}

	// value is not set, get the default value from the registry
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		"SOFTWARE\\Datadog\\Datadog Agent",
		registry.ALL_ACCESS)
	if err != nil {
		return
	}
	defer k.Close()
	val, _, err := k.GetStringValue("fleet_policies_dir")
	if err != nil {
		return
	}
	if val == "" {
		return
	}

	config.Set("fleet_policies_dir", val, pkgconfigmodel.SourceAgentRuntime)
}
