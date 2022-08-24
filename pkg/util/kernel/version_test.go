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

func TestParseDebianVersion(t *testing.T) {
	for _, tc := range []struct {
		succeed       bool
		releaseString string
		kernelVersion uint32
	}{
		// 4.9.168
		{true, "Linux version 4.9.0-9-amd64 (debian-kernel@lists.debian.org) (gcc version 6.3.0 20170516 (Debian 6.3.0-18+deb9u1) ) #1 SMP Debian 4.9.168-1+deb9u3 (2019-06-16)", 264616},
		// 4.9.88
		{true, "Linux ip-10-0-75-49 4.9.0-6-amd64 #1 SMP Debian 4.9.88-1+deb9u1 (2018-05-07) x86_64 GNU/Linux", 264536},
		// 3.0.4
		{true, "Linux version 3.16.0-9-amd64 (debian-kernel@lists.debian.org) (gcc version 4.9.2 (Debian 4.9.2-10+deb8u2) ) #1 SMP Debian 3.16.68-1 (2019-05-22)", 200772},
		// Invalid
		{false, "Linux version 4.9.125-linuxkit (root@659b6d51c354) (gcc version 6.4.0 (Alpine 6.4.0) ) #1 SMP Fri Sep 7 08:20:28 UTC 2018", 0},
		// 4.9.258-1 overflow of patch version which has max 255
		{true, "Linux version 4.9.0-15-amd64 (debian-kernel@lists.debian.org) (gcc version 6.3.0 20170516 (Debian 6.3.0-18+deb9u1) ) #1 SMP Debian 4.9.258-1 (2021-03-08)", 264703},
	} {
		version, err := parseDebianVersion(tc.releaseString)
		if err != nil && tc.succeed {
			t.Errorf("expected %q to succeed: %s", tc.releaseString, err)
		} else if err == nil && !tc.succeed {
			t.Errorf("expected %q to fail", tc.releaseString)
		}
		if version != Version(tc.kernelVersion) {
			t.Errorf("expected kernel version %d, got %d", tc.kernelVersion, version)
		}
	}
}

func TestParseUbuntuVersion(t *testing.T) {
	for _, tc := range []struct {
		succeed       bool
		procVersion   string
		kernelVersion uint32
	}{
		// 5.4.0-52.57
		{true, "Ubuntu 5.4.0-52.57-generic 5.4.65", 328769},
	} {
		version, err := parseUbuntuVersion(tc.procVersion)
		if err != nil && tc.succeed {
			t.Errorf("expected %q to succeed: %s", tc.procVersion, err)
		} else if err == nil && !tc.succeed {
			t.Errorf("expected %q to fail", tc.procVersion)
		}
		if version != Version(tc.kernelVersion) {
			t.Errorf("expected kernel version %d, got %d", tc.kernelVersion, version)
		}
	}
}
