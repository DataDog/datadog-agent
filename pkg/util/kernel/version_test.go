// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package kernel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLinuxKernelVersionCode(t *testing.T) {
	// Some sanity checks
	assert.Equal(t, VersionCode(2, 6, 9), Version(132617))
	assert.Equal(t, VersionCode(3, 2, 12), Version(197132))
	assert.Equal(t, VersionCode(4, 4, 0), Version(263168))

	assert.Equal(t, ParseVersion("2.6.9"), Version(132617))
	assert.Equal(t, ParseVersion("3.2.12"), Version(197132))
	assert.Equal(t, ParseVersion("4.4.0"), Version(263168))

	assert.Equal(t, Version(132617).String(), "2.6.9")
	assert.Equal(t, Version(197132).String(), "3.2.12")
	assert.Equal(t, Version(263168).String(), "4.4.0")
}
