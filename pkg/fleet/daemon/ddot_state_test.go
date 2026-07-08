// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDDOTExtensionInstalled(t *testing.T) {
	root := t.TempDir()

	// No DDOT extension present.
	assert.False(t, ddotExtensionInstalled(root))

	// otel-agent binary present under the stable datadog-agent package tree.
	binDir := filepath.Join(root, "datadog-agent", "stable", "ext", "ddot", "embedded", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "otel-agent"), []byte("#!/bin/sh\n"), 0o755))
	assert.True(t, ddotExtensionInstalled(root))
}
