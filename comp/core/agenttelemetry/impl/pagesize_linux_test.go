// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"encoding/binary"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// osPageSize returns AT_PAGESZ from /proc/self/auxv: the page size the kernel handed to this
// process, read without going through the Go runtime.
func osPageSize(t *testing.T) int {
	const atPagesz = 6

	auxv, err := os.ReadFile("/proc/self/auxv")
	require.NoError(t, err)

	// auxv is a list of (type, value) pairs of native words, so 4 bytes on 32-bit arm.
	wordSize := strconv.IntSize / 8
	word := func(b []byte) uint64 {
		if wordSize == 4 {
			return uint64(binary.NativeEndian.Uint32(b))
		}
		return binary.NativeEndian.Uint64(b)
	}
	for i := 0; i+2*wordSize <= len(auxv); i += 2 * wordSize {
		if word(auxv[i:]) == atPagesz {
			return int(word(auxv[i+wordSize:]))
		}
	}
	require.FailNow(t, "AT_PAGESZ not found in /proc/self/auxv")
	return 0
}
