// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package execinfomanager

import (
	"testing"

	tracertypes "github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/tracer/types"
	"github.com/stretchr/testify/require"
)

func TestBuildInterpreterLoadersSkipsGoTracer(t *testing.T) {
	var includeTracers tracertypes.IncludedTracers
	includeTracers.Enable(tracertypes.GoTracer)

	loaders := buildInterpreterLoaders(includeTracers)
	require.Len(t, loaders, 1)
}

func TestBuildInterpreterLoadersKeepsLabels(t *testing.T) {
	var includeTracers tracertypes.IncludedTracers
	includeTracers.Enable(tracertypes.Labels)

	loaders := buildInterpreterLoaders(includeTracers)
	require.Len(t, loaders, 2)
}
