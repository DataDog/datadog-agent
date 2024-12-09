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
// returns an error if this isn't the case
func VerifyAssetPermissions(assetPath string) error {
	// Enforce that we only load root-writeable object files
	info, err := os.Stat(assetPath)
	if err != nil {
		return fmt.Errorf("error stat-ing asset file %s: %w", assetPath, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("error getting permissions for output file %s", assetPath)
	}
	if stat.Uid != 0 || stat.Gid != 0 || info.Mode().Perm()&os.FileMode(0022) != 0 {
		return fmt.Errorf("%s has incorrect permissions: user=%v, group=%v, permissions=%v", assetPath, stat.Uid, stat.Gid, info.Mode().Perm())
	}
	return nil
}
