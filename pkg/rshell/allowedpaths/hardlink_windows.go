// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"io/fs"
)

// fileLinkCount always reports "unknown" on Windows.
//
// os.File.Stat backs FileInfo.Sys with *syscall.Win32FileAttributeData, which
// carries no link count; obtaining one would require a separate
// GetFileInformationByHandle call (nNumberOfLinks). Rather than add a new
// Windows syscall surface for a guard whose motivating exploit requires a
// pre-existing hard link inside a read-write root, the guard degrades to
// "not enforced" here. See the hard-link entry in AGENTS.md.
func fileLinkCount(_ fs.FileInfo) (uint64, bool) {
	return 0, false
}
