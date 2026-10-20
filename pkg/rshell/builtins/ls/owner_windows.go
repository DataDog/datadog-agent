// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ls

import (
	"context"
	"fmt"
	iofs "io/fs"
	"os"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// fileOwner returns the owner, group, and hard link count.
// On Windows, UID/GID do not exist so owner and group are returned as "?".
// Hard link count is resolved through the sandbox via
// GetFileInformationByHandle.
func fileOwner(ctx context.Context, callCtx *builtins.CallContext, path string, _ iofs.FileInfo) (owner, group, nlink string) {
	owner = "?"
	group = "?"
	nlink = "?"
	if n, ok := getNlink(ctx, callCtx, path); ok {
		nlink = fmt.Sprintf("%d", n)
	}
	return owner, group, nlink
}

// getNlink opens the file through the sandbox and queries
// GetFileInformationByHandle to retrieve the hard link count.
func getNlink(ctx context.Context, callCtx *builtins.CallContext, path string) (uint64, bool) {
	if path == "" || callCtx == nil || callCtx.OpenFile == nil {
		return 0, false
	}
	rc, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
	if err != nil {
		return 0, false
	}
	defer rc.Close()

	// The sandbox returns an *os.File wrapped as io.ReadWriteCloser.
	// Type-assert to get the file descriptor for the Windows API call.
	type fder interface{ Fd() uintptr }
	fd, ok := rc.(fder)
	if !ok {
		return 0, false
	}

	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(fd.Fd()), &d); err != nil {
		return 0, false
	}
	return uint64(d.NumberOfLinks), true
}

// fileBlocks returns the number of 512-byte blocks allocated for the file
// and whether the value is available. On Windows, GetFileInformationByHandle
// does not expose allocation size and GetFileInformationByHandleEx requires
// the unsafe package which is permanently banned.
func fileBlocks(_ context.Context, _ *builtins.CallContext, _ string, _ iofs.FileInfo) (int64, bool) {
	return 0, false
}
