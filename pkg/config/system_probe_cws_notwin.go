// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

func platformCWSConfig(cfg Config) {
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", DefaultRuntimePoliciesDir)
	cfg.BindEnvAndSetDefault("runtime_security_config.socket", "/opt/datadog-agent/run/runtime-security.sock")
}
