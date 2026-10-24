// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package allowedpaths

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveAllowedPathModeStripsWindowsSuffix(t *testing.T) {
	base := filepath.Join(t.TempDir(), "policy")

	path, mode := resolveAllowedPathMode(base + ":rw")
	assert.Equal(t, base, path)
	assert.Equal(t, pathModeReadWrite, mode)

	path, mode = resolveAllowedPathMode(base + ":ro")
	assert.Equal(t, base, path)
	assert.Equal(t, pathModeReadOnly, mode)
}
