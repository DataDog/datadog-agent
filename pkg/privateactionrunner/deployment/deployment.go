// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package deployment resolves the effective PAR deployment mode.
package deployment

import (
	"os"
	"runtime"

	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/fips"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
)

// Mode identifies the selected PAR deployment architecture.
type Mode string

const (
	// Monolithic runs PAR in the Go process, including unsupported split configurations.
	Monolithic Mode = "monolithic"
	// Split runs the Rust control plane with an on-demand Go executor.
	Split Mode = "split"
)

// Requested reports whether split deployment was explicitly selected.
func Requested(cfg model.Reader) bool {
	return requested(cfg.GetBool(par.SplitEnabled), configenv.IsContainerized(), os.Getenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"))
}

func requested(configEnabled, containerized bool, envValue string) bool {
	if containerized {
		return envValue == "true"
	}
	return configEnabled
}

// Resolve applies the same deployment policy to monolithic startup and the core startup gate.
// Callers must not start either architecture when resolution fails.
func Resolve(cfg model.Reader) (Mode, error) {
	return resolve(cfg, runtime.GOOS, configenv.IsContainerized(), os.Getenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"), fips.Enabled)
}

func resolve(cfg model.Reader, goos string, containerized bool, envValue string, fipsEnabled func() (bool, error)) (Mode, error) {
	if !requested(cfg.GetBool(par.SplitEnabled), containerized, envValue) {
		return Monolithic, nil
	}
	if cfg.GetBool("fips.enabled") || !(goos == "linux" || (goos == "windows" && !containerized)) {
		return Monolithic, nil
	}
	// Packaged host launchers opt out when their service layout does not start procmgr.
	// Containers explicitly launch their own supervisor instead.
	if !containerized && !cfg.GetBool("process_manager.enabled") {
		return Monolithic, nil
	}
	buildFIPS, err := fipsEnabled()
	if err != nil {
		return Monolithic, err
	}
	if buildFIPS {
		return Monolithic, nil
	}
	return Split, nil
}
