// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows
// +build cpython

package app

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	pythonBin = "python.exe"
)

var (
	relPyPath              = pythonBin
	relTufConfigFilePath   = filepath.Join("..", tufConfigFile)
	relChecksPath          = filepath.Join("Lib", "site-packages", "datadog_checks")
	relReqAgentReleasePath = filepath.Join("..", reqAgentReleaseFile)
	tufPipCachePath        = filepath.Join("c:\\", "ProgramData", "Datadog", "repositories", "cache")
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		tufPipCachePath = filepath.Join(pd, "Datadog", "repositories", "cache")
	} else {
		winutil.LogEventViewer(config.ServiceName, 0x8000000F, tufPipCachePath)
	}
}
func authorizedUser() bool {
	// TODO: implement something useful
	return true
}

func getTUFPipCachePath() (string, error) {
	if _, err := os.Stat(tufPipCachePath); err != nil {
		if os.IsNotExist(err) {
			return tufPipCachePath, err
		}
	}

	return tufPipCachePath, nil
}
