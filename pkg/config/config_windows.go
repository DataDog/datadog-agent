// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.datadoghq.com/).
// Copyright 2016-2019 StackState, Inc.

package config

import (
	"path/filepath"

	"github.com/StackVista/stackstate-agent/pkg/util/winutil"
)

var (
	defaultConfdPath            = "c:\\programdata\\datadog\\conf.d"
	defaultAdditionalChecksPath = "c:\\programdata\\datadog\\checks.d"
	defaultRunPath              = "c:\\programdata\\datadog\\run"
	defaultSyslogURI            = ""
	defaultGuiPort              = 5002
)

// ServiceName is the name that'll be used to register the Agent
const ServiceName = "StackStateAgent"

func osinit() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfdPath = filepath.Join(pd, "StackState", "conf.d")
		defaultAdditionalChecksPath = filepath.Join(pd, "StackState", "checks.d")
		defaultRunPath = filepath.Join(pd, "StackState", "run")
	} else {
		winutil.LogEventViewer(ServiceName, 0x8000000F, defaultConfdPath)
	}
}

// NewAssetFs  Should never be called on non-android
func setAssetFs(config Config) {}
