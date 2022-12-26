// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Merge will merge the security-agent configuration into the existing datadog configuration
func Merge(configPaths []string) error {
	// TODO: Unify with System Probe config merge fnc
	// TODO: This code is not actually hit at the moment due to the MergeConfigurationFiles fnc. This will be used post-fx refactor.
	for _, configPath := range configPaths {
		if f, err := os.Open(configPath); err == nil {
			configFileExtension := filepath.Ext(configPath)
			if !(configFileExtension == ".yaml" || configFileExtension == ".yml") {
				log.Warnf("Security Agent config file is not a yaml file. May not merge properly.")
			} else {
				log.Infof("Security Agent config setup is setting Datadog Agent config type to yaml.")
				config.Datadog.SetConfigType("yaml")
			}

			err = config.Datadog.MergeConfig(f)
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("error merging %s config file: %w", configPath, err)
			}
		} else {
			log.Infof("no config exists at %s, ignoring...", configPath)
		}
	}

	return nil
}
