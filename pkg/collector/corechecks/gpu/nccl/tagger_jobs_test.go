// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package nccl

import (
	"testing"

	"github.com/stretchr/testify/require"

	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

func TestSetJobsConfigDegradedMode(t *testing.T) {
	pt := NewProcessTagger(nil, nil, nil, nil)

	// SetJobsConfig on a nil cache must be a no-op (not panic)
	require.NotPanics(t, func() { pt.SetJobsConfig(gpuconfig.JobsConfig{}) })
}
