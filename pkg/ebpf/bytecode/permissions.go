// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package bytecode

import (
	"fmt"
	"os"
	"syscall"
)

// VerifyAssetPermissions checks that the file at the given path is owned by root,
// and does not have write permission for group and other;
// returns an error if this isn't the case.
//
// Prefer VerifyAssetPermissionsAndOpen when the file is going to be read: it runs
// the same check on the descriptor it returns, so the file that is checked is the
// file that is used.
func VerifyAssetPermissions(assetPath string) error {
	f, err := VerifyAssetPermissionsAndOpen(assetPath)
	if err != nil {
		return err
	}
	return f.Close()
}

// VerifyAssetPermissionsAndOpen opens the asset at the given path with
// O_NOFOLLOW and verifies, using the opened file descriptor, that the file is
// owned by root and is not writable by group or other. On success it returns the
// open *os.File so the caller can read the exact bytes that were verified: the
// check and the read use the same descriptor rather than resolving the path a
// second time. The caller owns the returned file and must close it.
func VerifyAssetPermissionsAndOpen(assetPath string) (*os.File, error) {
	f, err := os.OpenFile(assetPath, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("error opening asset file %s: %w", assetPath, err)
	}

	// Stat the descriptor (fstat), not the path, so the checked file is exactly
	// the one we hold open.
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("error stat-ing asset file %s: %w", assetPath, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		f.Close()
		return nil, fmt.Errorf("error getting permissions for output file %s", assetPath)
	}
	if err := verifyOwnerPermissions(assetPath, stat.Uid, stat.Gid, info.Mode().Perm()); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// verifyOwnerPermissions enforces that the given ownership and permission bits
// describe a root-owned file that is not writable by group or other. It is kept
// separate from the filesystem access above so the policy can be unit-tested
// with synthetic values, without needing to create root-owned files (which a
// test run as non-root cannot do).
func verifyOwnerPermissions(assetPath string, uid, gid uint32, perm os.FileMode) error {
	// Enforce that we only load root-owned, non-group/other-writable object files.
	if uid != 0 || gid != 0 || perm&os.FileMode(0022) != 0 {
		return fmt.Errorf("%s has incorrect permissions: user=%v, group=%v, permissions=%v", assetPath, uid, gid, perm)
	}
	return nil
}
