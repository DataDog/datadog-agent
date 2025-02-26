// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package python

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/stretchr/testify/require"
)

func TestGetPythonInfo(t *testing.T) {
	expected := "3.10.6 (main, May 29 2023, 11:10:38) [GCC 11.3.0]"
	cache.Cache.Set(pythonInfoCacheKey, expected, cache.NoExpiration)
	defer cache.Cache.Delete(pythonInfoCacheKey)
	require.Equal(t, expected, GetPythonInfo())
}

func TestGetPythonInfoNoSet(t *testing.T) {
	require.Equal(t, "n/a", GetPythonInfo())
}

func TestGetPythonVersion(t *testing.T) {
	cache.Cache.Set(pythonInfoCacheKey, "3.10.6 (main, May 29 2023, 11:10:38) [GCC 11.3.0]", cache.NoExpiration)
	defer cache.Cache.Delete(pythonInfoCacheKey)
	require.Equal(t, "3.10.6", GetPythonVersion())
}

func TestGetPythonVersionNotSet(t *testing.T) {
	require.Equal(t, "n/a", GetPythonVersion())
}
