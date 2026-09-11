// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package software

// Software types shared across platforms
const (
	// softwareTypeOS represents the running operating system.
	//
	// Individual updates are deliberately not collected. An update is an event rather
	// than an installed item whose version advances, which is what this inventory
	// models: a KB is installed once and never changes, so it would produce a
	// permanently static entry. The patch level is reported as the version of this
	// entry instead, and the host SBOM already carries the installed KBs.
	softwareTypeOS = "os"
	// softwareTypeDriver represents device driver packages
	softwareTypeDriver = "driver"
)

// The operating system is reported as a single entry whose version advances. Both the
// name and the product code are constant, because the backend keys a software item on
// name and product code with the version excluded: anything version-derived in either
// field would make an upgrade read as an uninstall followed by a fresh install.
const (
	osDisplayName = "OS"
	osProductCode = "os_version"
)
