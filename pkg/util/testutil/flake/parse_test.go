// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package flake

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFlaky(t *testing.T) {
	kf := &KnownFlakyTests{}
	kf.Add("", "TestEKSSuite/TestCPU")

	assert.True(t, kf.IsFlaky("", "TestEKSSuite/TestCPU/TestCPUUtilization"))
	assert.True(t, kf.IsFlaky("", "TestEKSSuite"))
	assert.False(t, kf.IsFlaky("", "TestEKSSuite/TestMemory"))
}
