// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package common

import (
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// SetupConfigForTest fires up the configuration system and returns warnings if any.
func SetupConfigForTest(confFilePath string) (*config.Warnings, error) {
	cfg := config.Datadog()
	origin := "datadog.yaml"
	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		cfg.AddConfigPath(confFilePath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(confFilePath, ".yaml") {
			cfg.SetConfigFile(confFilePath)
		}
	}
	cfg.AddConfigPath(path.DefaultConfPath)
	// load the configuration
	warnings, err := config.LoadDatadogCustom(cfg, origin, optional.NewNoneOption[secrets.Component](), nil)
	if err != nil {
		// special-case permission-denied with a clearer error message
		if errors.Is(err, fs.ErrPermission) {
			if runtime.GOOS == "windows" {
				err = fmt.Errorf(`cannot access the Datadog config file (%w); try running the command in an Administrator shell"`, err)
			} else {
				err = fmt.Errorf("cannot access the Datadog config file (%w); try running the command under the same user as the Datadog Agent", err)
			}
		} else {
			err = fmt.Errorf("unable to load Datadog config file: %w", err)
		}
		return warnings, err
	}
	return warnings, nil
}
