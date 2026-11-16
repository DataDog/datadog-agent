// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestAbstractBodyRangeSafety(t *testing.T) {
	for _, ranges := range [][]ir.PCRange{
		{{10, 20}, {10, 20}},
		{{10, 20}, {19, 30}},
	} {
		require.Error(t, validateNonOverlappingPCRanges(ranges, 1, "abstract subprogram bodies"))
	}
	require.NoError(t, validateNonOverlappingPCRanges([]ir.PCRange{{10, 20}, {20, 30}}, 1, "abstract subprogram bodies"))
}
