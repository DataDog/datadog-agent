// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Merge will merge additional configuration into an existing configuration. The default is merging security-agent.yaml into datadog.yaml.
func Merge(configPaths []string) error {
	for _, configPath := range configPaths {
		if f, err := os.Open(configPath); err == nil {
			err = pkgconfig.Datadog.MergeConfig(f)
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("error merging %s config file: %w", configPath, err)
			}
		} else {
			pkglog.Infof("no config exists at %s, ignoring...", configPath)
		}
	}

	return nil
}
