// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func platformCWSConfig(cfg Config) {
	programdata, err := winutil.GetProgramDataDir()
	if err == nil {
		cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", filepath.Join(programdata, "runtime-security.d"))
	} else {
		cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", "c:\\programdata\\datadog\\runtime-security.d")
	}
	cfg.BindEnvAndSetDefault("runtime_security_config.socket", "localhost:3334")
}
