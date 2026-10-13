// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"regexp"
	"strings"

	gopsutilhost "github.com/shirou/gopsutil/v4/host"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

type platformInfo struct {
	platform string
	family   string
	version  string
}

// Platform is the string describing the Linux distribution (ubuntu, debian, fedora, etc.)
var Platform = funcs.Memoize(func() (string, error) {
	pi, err := platformInformation()
	return pi.platform, err
})

// PlatformVersion is the string describing the platform version (`22.04` for Ubuntu jammy, etc.)
var PlatformVersion = funcs.Memoize(func() (string, error) {
	pi, err := platformInformation()
	return pi.version, err
})

// Family is the string describing the Linux distribution family (rhel, debian, etc.)
var Family = funcs.Memoize(func() (string, error) {
	pi, err := platformInformation()
	return pi.family, err
})

// platformCorrections is a set of invariants between kernel version patterns and platform info.
var platformCorrections = []struct {
	pattern          *regexp.Regexp
	info             platformInfo
	versionTransform func(string) string
	// ubuntu 20.04 is the most frequent incorrect platform because it comes from the DD container image
	ubuntuOnly bool
}{
	{pattern: regexp.MustCompile(`\.amzn([1-2])\.`), info: platformInfo{platform: "amazon", family: "rhel"}},
	{pattern: regexp.MustCompile(`\.amzn2023\.`), info: platformInfo{platform: "amazon", family: "rhel", version: "2023"}},
	{pattern: regexp.MustCompile(`\.el([7-8])uek\.`), info: platformInfo{platform: "oracle", family: "rhel"}, ubuntuOnly: true},
	// we don't know the actual platform between CentOS, RHEL, Rocky, etc. but we know it isn't Ubuntu. Set to RHEL as reasonable default.
	{pattern: regexp.MustCompile(`\.el(\d+(_\d+)?)\.`), info: platformInfo{platform: "redhat", family: "rhel"}, ubuntuOnly: true, versionTransform: func(v string) string {
		return strings.Replace(v, "_", ".", 1)
	}},
	{pattern: regexp.MustCompile(`[.+]deb(1[0-9])[.-]`), info: platformInfo{platform: "debian", family: "debian"}},
	{pattern: regexp.MustCompile(`\.fc(\d{2})\.`), info: platformInfo{platform: "fedora", family: "fedora"}},
	{pattern: regexp.MustCompile(`-lp(15\d)\.`), info: platformInfo{platform: "opensuse-leap", family: "suse"}, versionTransform: func(v string) string {
		return strings.Replace(v, "15", "15.", 1)
	}},
	// could be SLES or OpenSUSE-Leap
	{pattern: regexp.MustCompile(`-150300\.`), info: platformInfo{platform: "sles", family: "suse", version: "15.3"}, ubuntuOnly: true},
}

var platformInformation = funcs.Memoize(getPlatformInformation)

func getPlatformInformation() (platformInfo, error) {
	platform, family, version, err := gopsutilhost.PlatformInformation()
	info := platformInfo{platform, family, version}
	if err != nil {
		return info, err
	}
	v, err := Release()
	if err != nil {
		// ignore version error, return platform info
		return info, nil
	}
	return correctPlatform(info, v), nil
}

// Ensure kernel version matches platform information. This helps correct when containerized environments
// do not have the host /etc/os-release (or related) files correctly mounted into the container.
func correctPlatform(info platformInfo, kernelVersion string) platformInfo {
	if info.platform == "ol" {
		// gopsutil doesn't handle the ol->oracle alias
		info.platform, info.family = "oracle", "rhel"
	}

	for _, p := range platformCorrections {
		matches := p.pattern.FindStringSubmatch(kernelVersion)
		if matches == nil {
			continue
		}
		if p.info.platform == info.platform {
			// platform is already correct, return
			return info
		}
		if p.ubuntuOnly && info.platform != "ubuntu" {
			continue
		}

		if p.pattern.NumSubexp() == 0 {
			return p.info
		}
		if len(matches) > 1 {
			info = p.info
			if p.versionTransform != nil {
				info.version = p.versionTransform(matches[1])
			} else {
				info.version = matches[1]
			}
			return info
		}
	}
	return info
}
