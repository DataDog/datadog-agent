// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package utils is a compliance internal submodule implementing various utilities.
package utils

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// GetFileUser returns the file user.
func GetFileUser(fi os.FileInfo) string {
	if statt, ok := fi.Sys().(*syscall.Stat_t); ok {
		u := strconv.Itoa(int(statt.Uid))
		if user, err := user.LookupId(u); err == nil {
			return user.Username
		}
		return fmt.Sprintf("uid:%d", statt.Uid)
	}
	return ""
}

// GetFileGroup returns the file group.
func GetFileGroup(fi os.FileInfo) string {
	if statt, ok := fi.Sys().(*syscall.Stat_t); ok {
		g := strconv.Itoa(int(statt.Gid))
		if group, err := user.LookupGroupId(g); err == nil {
			return group.Name
		}
		return fmt.Sprintf("gid:%d", statt.Gid)
	}
	return ""
}
