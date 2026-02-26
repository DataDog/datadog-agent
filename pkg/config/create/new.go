// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package create provides the constructor for the config
package create

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/teeconfig"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// NewConfig returns a config with the given name. Implementation of the
// config is chosen by an env var
//
// Possible values for DD_CONF_NODETREEMODEL:
//   - "enable":          Use the nodetreemodel for the config, instead of viper
//   - "tee":             Construct both viper and nodetreemodel. Write to both, only read from viper (base=viper, compare=nodetreemodel)
//   - "enable-tee":      Same as tee but with base=nodetreemodel and compare=viper (read from nodetreemodel, log diffs vs viper)
//   - "<Agent version>": enable NTM if the Agent has a version equal or higher than the given version. This acts has a
//     minimum version for whitch to enable NTM, useful when using the same configuration across
//     different agent versions.
//   - other:             Use viper
func NewConfig(name string, configLib string) model.BuildableConfig {
	lib, ok := os.LookupEnv("DD_CONF_NODETREEMODEL")
	if !ok {
		lib = configLib
	}

	lib = strings.Trim(lib, " ")

	if lib == "enable" {
		return nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	} else if lib == "viper" {
		return viperconfig.NewViperConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	} else if lib == "tee" {
		viperImpl := viperconfig.NewViperConfig(name, "DD", strings.NewReplacer(".", "_"))         // nolint: forbidigo // legit use case
		nodetreeImpl := nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
		return teeconfig.NewTeeConfig(viperImpl, nodetreeImpl)
	} else if lib == "enable-tee" {
		viperImpl := viperconfig.NewViperConfig(name, "DD", strings.NewReplacer(".", "_"))         // nolint: forbidigo // legit use case
		nodetreeImpl := nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
		return teeconfig.NewTeeConfig(nodetreeImpl, viperImpl)
	} else if lib != "" {
		// If the Agent is at or above the minimum version specified we enabled NTM
		agentVersion, err := version.Agent()
		if err == nil {
			if res, err := agentVersion.CompareTo(lib); err == nil && res >= 0 {
				return nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
			}
		}
	}

	// Default case: enable-tee
	viperImpl := viperconfig.NewViperConfig(name, "DD", strings.NewReplacer(".", "_"))         // nolint: forbidigo // legit use case
	nodetreeImpl := nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	return teeconfig.NewTeeConfig(nodetreeImpl, viperImpl)
}
