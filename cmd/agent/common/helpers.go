// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package common

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// SetupConfig fires up the configuration system
func SetupConfig(confFilePath string) error {
	return setupConfig(confFilePath, false)
}

// SetupConfigWithoutSecrets fires up the configuration system without secrets support
func SetupConfigWithoutSecrets(confFilePath string) error {
	return setupConfig(confFilePath, true)
}

func setupConfig(confFilePath string, withoutSecrets bool) error {
	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(confFilePath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(confFilePath, ".yaml") {
			config.Datadog.SetConfigFile(confFilePath)
		}
	}
	config.Datadog.AddConfigPath(DefaultConfPath)
	// load the configuration
	var err error
	if withoutSecrets {
		err = config.LoadWithoutSecret()
	} else {
		err = config.Load()
	}
	if err != nil {
		return fmt.Errorf("unable to load Datadog config file: %s", err)
	}
	return nil
}
