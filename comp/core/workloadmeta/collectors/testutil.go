// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && linux && nvml

// Package collectors contains collectors for the workloadmeta component
package collectors

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/nvml"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// GetNvmlCollector creates the NVML collector, exposed here for testing purposes only
func GetNvmlCollector(t *testing.T, config config.Component) workloadmeta.Collector {
	lc := fxtest.NewLifecycle(t)
	collector, err := nvml.NewCollector(config, lc)
	require.NoError(t, err, "failed to create NVML collector")
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)
	return collector.Collector
}
