// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"io/fs"
	"os"
	"syscall"
)

// fileLinkCount always reports "unknown" on Windows.
//
// os.File.Stat and os.Root.Lstat back FileInfo.Sys with
// *syscall.Win32FileAttributeData, which carries no link count. Callers that
// hold an open descriptor must use openFileLinkCount instead, which queries
// the handle directly; this path-metadata variant is only used by advisory
// pre-checks that defer to the descriptor-based guard at execution time.
func fileLinkCount(_ fs.FileInfo) (uint64, bool) {
	return 0, false
}

// openFileLinkCount returns the NTFS hard-link count (nNumberOfLinks) of the
// already-open file f via GetFileInformationByHandle. The query runs on the
// exact handle the caller is about to mutate, so it cannot be raced by a
// concurrent rename. Go's own os.File.Stat issues the same call on the same
// handle, so any handle that could be stat'ed can be queried here.
func openFileLinkCount(f *os.File, _ fs.FileInfo) (uint64, error) {
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d); err != nil {
		return 0, err
	}
	return uint64(d.NumberOfLinks), nil
}
