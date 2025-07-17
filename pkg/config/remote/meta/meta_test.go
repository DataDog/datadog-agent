// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package meta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseProdDirectorVersion(t *testing.T) {
	v, err := parseRootVersion(prodRootDirector)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseProdTUFVersion(t *testing.T) {
	v, err := parseRootVersion(prodRootConfig)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseStagingDirectorVersion(t *testing.T) {
	v, err := parseRootVersion(stagingRootDirector)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseStagingTUFVersion(t *testing.T) {
	v, err := parseRootVersion(stagingRootConfig)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}
