// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
)

func SupportedBuildModes(t *testing.T, modes []ebpftest.BuildMode) []ebpftest.BuildMode {
	if err := offsetguess.IsTracerOffsetGuessingSupported(); err == offsetguess.ErrTracerOffsetGuessingNotSupported {
		t.Log("removing pre-compiled build mode as it is not supported on this platform")
		modes = slices.DeleteFunc(modes, func(bm ebpftest.BuildMode) bool {
			return bm == ebpftest.Prebuilt
		})
	} else {
		require.NoError(t, err, "error determining if tracer offset guessing is supported")
	}

	return modes
}
