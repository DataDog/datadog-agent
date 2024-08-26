// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package flake

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFlaky(t *testing.T) {
	kf := &KnownFlakyTests{}
	kf.Add("", "TestEKSSuite/TestCPU")

	assert.True(t, kf.IsFlaky("", "TestEKSSuite/TestCPU/TestCPUUtilization"))
	// parents cannot be considered flaky without test results to ensure it may have failed because of a flaky subtest
	assert.False(t, kf.IsFlaky("", "TestEKSSuite"))
	assert.False(t, kf.IsFlaky("", "TestEKSSuite/TestMemory"))
	assert.True(t, kf.IsFlaky("", "TestEKSSuite/TestCPU"))
	assert.False(t, kf.IsFlaky("", "TestECSSuite/TestCPU"))
}

const flake1 = `pkg/gohai:
  - TestGetPayload`

const flake2 = `pkg/toto:
  - TestGetPayload
  - TestOtherTest`

const flake3 = `pkg/gohai:
  - TestGetPayload
pkg/toto:
  - TestGetPayload
  - TestOtherTest`

func TestFlakesParse(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		kf, err := Parse(bytes.NewBuffer([]byte(flake1)))
		require.NoError(t, err)
		if assert.Contains(t, kf.packageTestList, "pkg/gohai") {
			assert.Contains(t, kf.packageTestList["pkg/gohai"], "TestGetPayload")
		}
	})

	t.Run("2", func(t *testing.T) {
		kf, err := Parse(bytes.NewBuffer([]byte(flake2)))
		require.NoError(t, err)
		if assert.Contains(t, kf.packageTestList, "pkg/toto") {
			assert.Contains(t, kf.packageTestList["pkg/toto"], "TestGetPayload")
			assert.Contains(t, kf.packageTestList["pkg/toto"], "TestOtherTest")
		}
	})

	t.Run("3", func(t *testing.T) {
		kf, err := Parse(bytes.NewBuffer([]byte(flake3)))
		require.NoError(t, err)
		if assert.Contains(t, kf.packageTestList, "pkg/gohai") {
			assert.Contains(t, kf.packageTestList["pkg/gohai"], "TestGetPayload")
		}
		if assert.Contains(t, kf.packageTestList, "pkg/toto") {
			assert.Contains(t, kf.packageTestList["pkg/toto"], "TestGetPayload")
			assert.Contains(t, kf.packageTestList["pkg/toto"], "TestOtherTest")
		}
	})
}
