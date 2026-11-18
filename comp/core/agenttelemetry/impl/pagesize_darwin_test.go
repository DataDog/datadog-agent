// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// osPageSize returns hw.pagesize: 16KiB on Apple silicon and 4KiB on Intel.
func osPageSize(t *testing.T) int {
	pageSize, err := syscall.SysctlUint32("hw.pagesize")
	require.NoError(t, err)
	return int(pageSize)
}
