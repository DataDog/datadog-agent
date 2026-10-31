// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "remove-me")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, RemoveAll(t.Context(), dir))
	assert.NoDirExists(t, dir)
}
