// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

func TestDefaultArtifactStoreRoot(t *testing.T) {
	root, err := defaultArtifactStoreRoot()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(defaultpaths.GetDefaultRunPath(), authoredScriptCacheDirectory), root)
}
