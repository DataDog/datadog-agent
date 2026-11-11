// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package du

import (
	iofs "io/fs"
	"syscall"
)

// infoBlocks returns the number of statBlockUnit-sized blocks (512 bytes
// each) actually allocated for the file. Returns false when Stat_t is
// unavailable (e.g. virtual filesystems on some platforms).
func infoBlocks(info iofs.FileInfo) (int64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int64(st.Blocks), true
}
