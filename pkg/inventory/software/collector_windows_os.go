// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// microsoftPublisher is the publisher reported for the operating system
const microsoftPublisher = "Microsoft Corporation"

// osCollector reports the running Windows installation as a single entry.
type osCollector struct {
	// versionFn returns the full OS version. It is a field so tests do not depend on
	// the version of the host they run on; nil means read the real value.
	versionFn func() (string, error)
}

// Collect returns one entry for the running operating system.
//
// Failing to determine the version is fatal: a host always runs some version of Windows,
// so reporting nothing would read downstream as the operating system having been removed.
func (c *osCollector) Collect() ([]*Entry, []*Warning, error) {
	version := c.versionFn
	if version == nil {
		version = winutil.GetWindowsVersion
	}

	osVersion, err := version()
	if err != nil {
		return nil, nil, err
	}

	return []*Entry{{
		Source:      softwareTypeOS,
		DisplayName: osDisplayName,
		Version:     osVersion,
		Publisher:   microsoftPublisher,
		Status:      "installed",
		ProductCode: osProductCode,
		Is64Bit:     runtime.GOARCH != "386",
		// InstallDate is left empty: the registry records an install time for the
		// original installation, which is not when the current version was applied.
	}}, nil, nil
}
