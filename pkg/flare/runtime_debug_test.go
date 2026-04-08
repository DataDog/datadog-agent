// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

func TestGetRuntimeDebugInfo(t *testing.T) {
	data, err := getRuntimeDebugInfo()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	content := string(data)
	assert.Contains(t, content, "=== Build Info ===")
	assert.Contains(t, content, "=== GC Settings ===")
	assert.Contains(t, content, "=== GC Stats ===")
	assert.Contains(t, content, "GOGC:")
	assert.Contains(t, content, "GOMEMLIMIT:")
	assert.Contains(t, content, "Num GC:")
	assert.Contains(t, content, "Pause Total:")
}

func TestProvideRuntimeDebugInfo(t *testing.T) {
	mock := flarehelpers.NewFlareBuilderMock(t, false)
	err := provideRuntimeDebugInfo(mock)
	require.NoError(t, err)

	mock.AssertFileExists("runtime_debug_info.log")
}
