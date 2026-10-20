// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package du

import (
	iofs "io/fs"
)

// infoBlocks always returns false on Windows: the standard
// FileInfo.Sys() exposes Win32FileAttributeData which lacks an
// allocation-size field, and GetFileInformationByHandleEx requires
// `unsafe`, which is permanently banned by the symbol allowlist. Callers
// fall back to the apparent-size approximation in entrySize().
func infoBlocks(_ iofs.FileInfo) (int64, bool) {
	return 0, false
}
