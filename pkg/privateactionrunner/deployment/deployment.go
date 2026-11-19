// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package deployment resolves the effective PAR deployment mode.
package deployment

import (
	"errors"
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

// ErrFIPSUnsupported is returned when PAR is started in a FIPS environment.
var ErrFIPSUnsupported = errors.New("private action runner is not supported in FIPS mode")

// EnsureSupported returns ErrFIPSUnsupported when the Agent runs in FIPS mode,
// through a FIPS build, runtime FIPS mode or fips.enabled.
func EnsureSupported(cfg model.Reader) error {
	return ensureSupported(cfg, fips.BuiltForFIPS, fips.Enabled)
}

func ensureSupported(cfg model.Reader, builtForFIPS func() bool, fipsEnabled func() (bool, error)) error {
	if cfg.GetBool("fips.enabled") {
		return ErrFIPSUnsupported
	}
	if builtForFIPS() {
		return ErrFIPSUnsupported
	}
	enabled, err := fipsEnabled()
	if err != nil {
		return err
	}
	if enabled {
		return ErrFIPSUnsupported
	}
	return nil
}

// Resolve applies the same deployment policy to monolithic startup and the core startup gate.
// Callers must not start either architecture when resolution fails.
func Resolve(cfg model.Reader) (Mode, error) {
	return resolve(cfg, runtime.GOOS, configenv.IsContainerized(), os.Getenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"), fips.BuiltForFIPS, fips.Enabled)
}

func resolve(cfg model.Reader, goos string, containerized bool, envValue string, builtForFIPS func() bool, fipsEnabled func() (bool, error)) (Mode, error) {
	if err := ensureSupported(cfg, builtForFIPS, fipsEnabled); err != nil {
		return Monolithic, err
	}
	if !requested(cfg.GetBool(par.SplitEnabled), containerized, envValue) {
		return Monolithic, nil
	}
	if !(goos == "linux" || (goos == "windows" && !containerized && cfg.GetBool("process_manager.enabled"))) {
		return Monolithic, nil
	}
	return Split, nil
}
