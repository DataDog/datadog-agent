// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	defaultConfdPath            = "c:\\programdata\\datadog\\conf.d"
	defaultAdditionalChecksPath = "c:\\programdata\\datadog\\checks.d"
	defaultRunPath              = "c:\\programdata\\datadog\\run"
	defaultSyslogURI            = ""
	defaultGuiPort              = "5002"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfdPath = filepath.Join(pd, "Datadog", "conf.d")
		defaultAdditionalChecksPath = filepath.Join(pd, "Datadog", "checks.d")
		defaultRunPath = filepath.Join(pd, "Datadog", "run")
	}
}

// NewAssetFs  Should never be called on non-android
func setAssetFs(config Config) {}
