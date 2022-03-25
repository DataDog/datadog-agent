// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package kernel

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"

	"golang.org/x/sys/unix"
)

var versionRegex = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?.*$`)

// Version is a numerical representation of a kernel version
type Version uint32

var hostVersion Version

// String returns a string representing the version in x.x.x format
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch())
}

// Major returns the major number of the version code
func (v Version) Major() uint8 {
	return (uint8)(v >> 16)
}

// Minor returns the minor number of the version code
func (v Version) Minor() uint8 {
	return (uint8)((v >> 8) & 0xff)
}

// Patch returns the patch number of the version code
func (v Version) Patch() uint8 {
	return (uint8)(v & 0xff)
}

// HostVersion returns the running kernel version of the host
func HostVersion() (Version, error) {
	if hostVersion != 0 {
		return hostVersion, nil
	}

	if procVersion, err := ioutil.ReadFile("/proc/version_signature"); err == nil {
		v, err := parseUbuntuVersion(string(procVersion))
		if err != nil {
			return 0, fmt.Errorf("error parsing ubuntu kernel version: %s", err)
		}
		return v, nil
	}

	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return 0, fmt.Errorf("error calling uname: %w", err)
	}

	if v, err := parseDebianVersion(unix.ByteSliceToString(uname.Version[:])); err == nil {
		return v, nil
	}
	return ParseReleaseString(unix.ByteSliceToString(uname.Release[:]))
}

// ParseVersion parses a string in the format of x.x.x to a Version
func ParseVersion(s string) Version {
	var a, b, c byte
	fmt.Sscanf(s, "%d.%d.%d", &a, &b, &c)
	return VersionCode(a, b, c)
}

// VersionCode returns a Version computed from the individual parts of a x.x.x version
func VersionCode(major, minor, patch byte) Version {
	// KERNEL_VERSION(a,b,c) = (a << 16) + (b << 8) + (c)
	// Per https://github.com/torvalds/linux/blob/db7c953555388571a96ed8783ff6c5745ba18ab9/Makefile#L1250
	return Version((uint32(major) << 16) + (uint32(minor) << 8) + uint32(patch))
}

// ParseReleaseString converts a release string with format
// 4.4.2[-1] to a kernel version number in LINUX_VERSION_CODE format.
// That is, for kernel "a.b.c", the version number will be (a<<16 + b<<8 + c)
func ParseReleaseString(releaseString string) (Version, error) {
	versionParts := versionRegex.FindStringSubmatch(releaseString)
	if len(versionParts) < 3 {
		return 0, fmt.Errorf("got invalid release version %q (expected format '4.3.2-1')", releaseString)
	}
	var major, minor, patch uint64
	var err error
	major, err = strconv.ParseUint(versionParts[1], 10, 8)
	if err != nil {
		return 0, err
	}

	minor, err = strconv.ParseUint(versionParts[2], 10, 8)
	if err != nil {
		return 0, err
	}

	// patch is optional
	if len(versionParts) >= 4 {
		patch, _ = strconv.ParseUint(versionParts[3], 10, 8)
	}

	// clamp patch/sublevel to 255 EARLY in 4.14.252 because they merged this too early:
	// https://github.com/torvalds/linux/commit/e131e0e880f942f138c4b5e6af944c7ddcd7ec96
	if major == 4 && minor == 14 && patch >= 252 {
		patch = 255
	}
	// https://github.com/torvalds/linux/commit/a256aac5b7000bdf1232ed2bbd674582c0ab27ec
	if major == 4 && minor == 19 && patch >= 222 {
		patch = 255
	}

	return VersionCode(byte(major), byte(minor), byte(patch)), nil
}

func parseUbuntuVersion(procVersion string) (Version, error) {
	var u1, u2, releaseString string
	_, err := fmt.Sscanf(procVersion, "%s %s %s", &u1, &u2, &releaseString)
	if err != nil {
		return 0, err
	}
	return ParseReleaseString(releaseString)
}

var debianVersionRegex = regexp.MustCompile(`.* SMP Debian (\d+\.\d+.\d+-\d+)(?:\+[[:alnum:]]*)?.*`)

func parseDebianVersion(str string) (Version, error) {
	match := debianVersionRegex.FindStringSubmatch(str)
	if len(match) != 2 {
		return 0, fmt.Errorf("failed to parse kernel version from /proc/version: %s", str)
	}
	return ParseReleaseString(match[1])
}
