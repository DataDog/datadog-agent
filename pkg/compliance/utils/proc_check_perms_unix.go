// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package utils

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func checkFilePermissions(fi os.FileInfo, uids, gids []uint32) error {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("failed to get file info")
	}
	perms := fi.Mode().Perm()
	euid := uids[1] // effective UID
	egid := gids[1] // effective GID
	fileUID := stat.Uid
	fileGID := stat.Gid
	canRead := false
	if fileUID == 0 && perms&0004 == 0 {
		canRead = false // do not allow to read non world-readable files owned by root
	} else if euid == 0 {
		canRead = true
	} else if fileUID == euid {
		canRead = perms&0400 != 0
	} else if fileGID == egid {
		canRead = perms&0040 != 0 // NOTE: does not check supplementary groups
	} else {
		canRead = perms&0004 != 0
	}
	if !canRead {
		return fmt.Errorf(
			"file %s is not readable by process (euid=%d, egid=%d, file_uid=%d, file_gid=%d, perms=%04o)",
			fi.Name(), euid, egid, fileUID, fileGID, perms,
		)
	}
	return nil
}
