// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

// Package cpuoscillation provides a stub implementation for unsupported platforms.
// Per-container CPU oscillation detection requires Linux cgroups for CPU metrics.
package cpuoscillation

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "cpu_oscillation"
)

// Factory creates a new check factory (returns None on unsupported platforms)
func Factory(_ workloadmeta.Component, _ tagger.Component) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
