// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package loader

import (
	"crypto/rand"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// Verify that the RuntimeStats struct has the same layout as the stats struct
// used in C.
func TestRuntimeStatsHasSameLayoutAsStats(t *testing.T) {
	require.Equal(t, unsafe.Sizeof(RuntimeStats{}), unsafe.Sizeof(stats{}))
	bytes := make([]byte, unsafe.Sizeof(stats{}))
	rand.Read(bytes)
	cStats := *(*stats)(unsafe.Pointer(&bytes[0]))
	runtimeStats := RuntimeStats{
		CPU:          time.Duration(cStats.Cpu_ns),
		HitCnt:       cStats.Hit_cnt,
		ThrottledCnt: cStats.Throttled_cnt,
	}
	require.Equal(t, runtimeStats, *(*RuntimeStats)(unsafe.Pointer(&cStats)))
	require.Equal(t, cStats, *(*stats)(unsafe.Pointer(&runtimeStats)))
}
