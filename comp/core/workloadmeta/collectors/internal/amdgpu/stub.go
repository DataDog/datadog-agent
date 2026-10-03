// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

// Package amdgpu implements the workloadmeta collector for AMD GPUs, which
// reports GPU entities read from the amdgpu driver's sysfs attributes.
package amdgpu

import "go.uber.org/fx"

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return nil
}
