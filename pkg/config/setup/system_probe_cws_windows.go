// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package setup

import (
	"path/filepath"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func platformCWSConfig(cfg pkgconfigmodel.Setup) {
	programdata, err := winutil.GetProgramDataDir()
	if err == nil {
		cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", filepath.Join(programdata, "runtime-security.d"))
	} else {
		cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", "c:\\programdata\\datadog\\runtime-security.d")
	}
	cfg.BindEnvAndSetDefault("runtime_security_config.socket", "localhost:3335")
	cfg.BindEnvAndSetDefault("runtime_security_config.cmd_socket", "")
}
