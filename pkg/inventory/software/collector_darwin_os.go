// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"fmt"
	"strings"
)

// systemVersionPlistPath records the version of the running system. It is a variable so
// tests can point it at a fixture.
//
// This is the file sw_vers reads, so it is the primary source rather than a view over
// one, and unlike sw_vers it needs no subprocess and exposes the build as well.
var systemVersionPlistPath = "/System/Library/CoreServices/SystemVersion.plist"

// applePublisher is the publisher reported for the operating system
const applePublisher = "Apple Inc."

// osCollector reports the running macOS installation as a single entry.
type osCollector struct{}

// Collect returns one entry for the running operating system.
//
// Failing to read the system version is fatal: macOS always runs some version, so
// reporting nothing would read downstream as the operating system having been removed.
func (c *osCollector) Collect() ([]*Entry, []*Warning, error) {
	systemVersion, err := readPlistFile(systemVersionPlistPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read %s: %w", systemVersionPlistPath, err)
	}

	productVersion := strings.TrimSpace(systemVersion["ProductVersion"])
	if productVersion == "" {
		return nil, nil, fmt.Errorf("no ProductVersion in %s", systemVersionPlistPath)
	}

	return []*Entry{{
		Source:      softwareTypeOS,
		DisplayName: osDisplayName,
		Version:     osVersionString(productVersion, systemVersion["ProductBuildVersion"]),
		Publisher:   applePublisher,
		Status:      statusInstalled,
		ProductCode: osProductCode,
		Is64Bit:     true,
		// InstallDate is left empty: nothing records when the running version was applied.
	}}, nil, nil
}

// osVersionString combines the product version and the build into the version reported
// for the operating system, e.g. "15.6 (24G84)".
//
// The build is included because the product version alone does not identify a patch
// level: Apple ships supplemental updates and hardware-specific builds that advance the
// build while leaving the product version unchanged, and a Rapid Security Response
// advances it too. Apple's own release notes use this form, and keeping the build in
// parentheses rather than as a fourth dot-separated component matters — the build is not
// numeric, so a dotted form would look orderable while comparing wrongly.
func osVersionString(productVersion string, buildVersion string) string {
	build := strings.TrimSpace(buildVersion)
	if build == "" {
		return productVersion
	}
	return productVersion + " (" + build + ")"
}
