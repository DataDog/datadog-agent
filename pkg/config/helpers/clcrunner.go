// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helpers provides helper functions for the config package.
package helpers

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

// IsCLCRunner returns whether the Agent is in cluster check runner mode
func IsCLCRunner(config pkgconfigmodel.Reader) bool {
	if !config.GetBool("clc_runner_enabled") {
		return false
	}

	var cps []pkgconfigsetup.ConfigurationProviders
	if err := structure.UnmarshalKey(config, "config_providers", &cps); err != nil {
		return false
	}

	for _, name := range config.GetStringSlice("extra_config_providers") {
		cps = append(cps, pkgconfigsetup.ConfigurationProviders{Name: name})
	}

	// A cluster check runner is an Agent configured to run clusterchecks only
	// We want exactly one ConfigProvider named clusterchecks
	if len(cps) == 0 {
		return false
	}

	for _, cp := range cps {
		if cp.Name != "clusterchecks" {
			return false
		}
	}

	return true
}
