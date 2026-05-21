// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcmgrDaemonAt(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, ProcmgrDaemonRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o755))
	require.NoError(t, os.WriteFile(bin, nil, 0o755))
	assert.True(t, ProcmgrDaemonAt(root))
	assert.False(t, ProcmgrDaemonAt(t.TempDir()))
	assert.False(t, ProcmgrDaemonAt(""))
}
