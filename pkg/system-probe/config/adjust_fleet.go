// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

func adjustFleet(cfg model.Config) error {
	// Apply overrides for local config options
	setup.FleetConfigOverride(cfg)

	// Load the remote configuration
	fleetPoliciesDirPath := cfg.GetString("fleet_policies_dir")
	if fleetPoliciesDirPath != "" {
		err := cfg.MergeFleetPolicy(path.Join(fleetPoliciesDirPath, "system-probe.yaml"))
		if err != nil {
			return err
		}
	}

	return nil
}