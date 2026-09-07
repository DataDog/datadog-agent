// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procfs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetMappedFilesWithTruncation(t *testing.T) {
	files, truncated, err := GetMappedFilesWithTruncation(int32(os.Getpid()), 1, nil)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.True(t, truncated)
}
