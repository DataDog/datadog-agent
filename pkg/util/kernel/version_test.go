// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

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

func TestUbuntuKernelVersion(t *testing.T) {
	ubuntuVersion := "5.13.0-35-generic-lpae"
	ukv, err := NewUbuntuKernelVersion(ubuntuVersion)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, ukv.Major, 5)
	assert.Equal(t, ukv.Minor, 13)
	assert.Equal(t, ukv.Patch, 0)
	assert.Equal(t, ukv.Abi, 35)
	assert.Equal(t, ukv.Flavor, "generic-lpae")
}

var testData = []struct {
	succeed       bool
	releaseString string
	kernelVersion uint32
}{
	{true, "4.1.2-3", 262402},
	{true, "4.8.14-200.fc24.x86_64", 264206},
	{true, "4.1.2-3foo", 262402},
	{true, "4.1.2foo-1", 262402},
	{true, "4.1.2-rkt-v1", 262402},
	{true, "4.1.2rkt-v1", 262402},
	{true, "4.1.2-3 foo", 262402},
	{false, "foo 4.1.2-3", 0},
	{true, "4.1.2", 262402},
	{false, ".4.1.2", 0},
	{false, "4", 0},
	{false, "4.", 0},
	{true, "4.1.", 262400},
	{true, "4.1", 262400},
	{true, "4.19-ovh", 267008},
	{true, "4.14.252", 265983},
}

func TestParseReleaseString(t *testing.T) {
	for _, test := range testData {
		version, err := ParseReleaseString(test.releaseString)
		if err != nil && test.succeed {
			t.Errorf("expected %q to succeed: %s", test.releaseString, err)
		} else if err == nil && !test.succeed {
			t.Errorf("expected %q to fail", test.releaseString)
		}
		if version != Version(test.kernelVersion) {
			t.Errorf("expected kernel version %d, got %d", test.kernelVersion, version)
		}
	}
}
