// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestUbuntuKernelsNotSupported(t *testing.T) {
	for i := byte(114); i < byte(128); i++ {
		ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, i), "ubuntu", nil)
		assert.False(t, ok)
		assert.NotEmpty(t, msg)
	}

	for i := byte(100); i < byte(114); i++ {
		ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, i), "ubuntu", nil)
		assert.True(t, ok)
		assert.Empty(t, msg)
	}

	for i := byte(128); i < byte(255); i++ {
		ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, i), "ubuntu", nil)
		assert.True(t, ok)
		assert.Empty(t, msg)
	}
}

func TestExcludedKernelVersion(t *testing.T) {
	exclusionList := []string{"5.5.1", "6.3.2"}
	ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, 121), "ubuntu", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(5, 5, 1), "debian", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(6, 3, 2), "debian", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(6, 3, 1), "debian", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(5, 5, 2), "debian", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(3, 10, 0), "Linux-3.10.0-957.5.1.el7.x86_64-x86_64-with-centos-7.6.1810-Core", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)
}
