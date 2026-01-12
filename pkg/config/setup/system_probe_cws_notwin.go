// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package setup

import (
	"path/filepath"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func platformCWSConfig(cfg pkgconfigmodel.Setup) {
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", DefaultRuntimePoliciesDir)
	cfg.BindEnvAndSetDefault("runtime_security_config.socket", filepath.Join(InstallPath, "run/runtime-security.sock"))
	cfg.BindEnvAndSetDefault("runtime_security_config.cmd_socket", "")
}
