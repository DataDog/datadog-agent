// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/fips"
)

// SetPrivateActionRunnerDefault limits Linux default-on to packaged host split launches.
// Recompute after fleet policies and CLI overrides; explicit PAR settings always win.
func SetPrivateActionRunnerDefault(config model.Config) {
	setPrivateActionRunnerDefault(config, runtime.GOOS, env.IsContainerized(), os.Getenv("DD_INTERNAL_PAR_SPLIT_LAUNCH"), fips.Enabled)
}

func setPrivateActionRunnerDefault(config model.Config, goos string, containerized bool, launch string, fipsEnabled func() (bool, error)) {
	if goos != "linux" {
		return
	}
	enabled := false
	// The config flag alone does not identify a service layout that actually launches procmgr.
	if launch == "true" && !containerized && config.GetBool("private_action_runner.split_enabled") &&
		config.GetBool("process_manager.enabled") && !config.GetBool("fips.enabled") {
		buildFIPS, err := fipsEnabled()
		enabled = err == nil && !buildFIPS
	}
	config.Set("private_action_runner.enabled", enabled, model.SourceDefault)
}
