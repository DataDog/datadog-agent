// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package ls

import (
	"context"
	iofs "io/fs"
	"strconv"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// fileOwner returns the numeric UID, GID, and hard link count for the given
// FileInfo. Names are not resolved to avoid reading /etc/passwd or triggering
// NSS/LDAP lookups outside the sandbox.
// The ctx, callCtx, and path parameters are used on Windows to open files
// through the sandbox for GetFileInformationByHandle; on Unix they are ignored.
func fileOwner(_ context.Context, _ *builtins.CallContext, _ string, info iofs.FileInfo) (owner, group, nlink string) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "?", "?", "?"
	}

	owner = strconv.FormatUint(uint64(st.Uid), 10)
	group = strconv.FormatUint(uint64(st.Gid), 10)
	nlink = strconv.FormatUint(uint64(st.Nlink), 10)
	return owner, group, nlink
}

// fileBlocks returns the number of 512-byte blocks allocated for the file
// and whether the value is available. The ctx, callCtx, and path parameters
// are used on Windows; on Unix they are ignored (Stat_t has everything).
func fileBlocks(_ context.Context, _ *builtins.CallContext, _ string, info iofs.FileInfo) (int64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Blocks, true
}
