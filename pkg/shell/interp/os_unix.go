// Copyright (c) 2017, Andrey Nering <andrey.nering@gmail.com>
// See LICENSE for licensing information

//go:build unix

package interp

import (
	"context"
	"io/fs"
	"os/user"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
	"mvdan.cc/sh/v3/syntax"
)

func mkfifo(path string, mode uint32) error {
	return unix.Mkfifo(path, mode)
}

// access is similar to checking the permission bits from [io/fs.FileInfo],
// but it also takes into account the current user's role.
func (r *Runner) access(ctx context.Context, path string, mode uint32) error {
	// TODO(v4): "access" may need to become part of a handler, like "open" or "stat".
	return unix.Access(path, mode)
}

// unTestOwnOrGrp implements the -O and -G unary tests. If the file does not
// exist, or the current user cannot be retrieved, returns false.
func (r *Runner) unTestOwnOrGrp(ctx context.Context, op syntax.UnTestOperator, x string) bool {
	info, err := r.stat(ctx, x)
	if err != nil {
		return false
	}
	u, err := user.Current()
	if err != nil {
		return false
	}
	if op == syntax.TsUsrOwn {
		uid, _ := strconv.Atoi(u.Uid)
		return uint32(uid) == info.Sys().(*syscall.Stat_t).Uid
	}
	gid, _ := strconv.Atoi(u.Gid)
	return uint32(gid) == info.Sys().(*syscall.Stat_t).Gid
}

type waitStatus = syscall.WaitStatus

// lsFileOwnership extracts ownership and link info from a FileInfo on Unix.
func lsFileOwnership(info fs.FileInfo, numeric bool) (owner, group string, nlink uint64, inode uint64) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "0", "0", 1, 0
	}
	nlink = uint64(st.Nlink)
	inode = st.Ino

	if numeric {
		owner = strconv.FormatUint(uint64(st.Uid), 10)
		group = strconv.FormatUint(uint64(st.Gid), 10)
	} else {
		if u, err := user.LookupId(strconv.FormatUint(uint64(st.Uid), 10)); err == nil {
			owner = u.Username
		} else {
			owner = strconv.FormatUint(uint64(st.Uid), 10)
		}
		if g, err := user.LookupGroupId(strconv.FormatUint(uint64(st.Gid), 10)); err == nil {
			group = g.Name
		} else {
			group = strconv.FormatUint(uint64(st.Gid), 10)
		}
	}
	return
}

// lsFileBlocks returns the number of 512-byte blocks allocated for the file.
func lsFileBlocks(info fs.FileInfo) int64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return st.Blocks
}
