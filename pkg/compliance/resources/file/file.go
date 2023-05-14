// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package file

import (
	"errors"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func getFileStatt(fi os.FileInfo) (*syscall.Stat_t, error) {
	statt, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("expected to get stat_t from fileinfo")
	}
	return statt, nil
}

func getFileUser(fi os.FileInfo) (string, error) {
	statt, err := getFileStatt(fi)
	if err != nil {
		return "", nil
	}
	u := strconv.Itoa(int(statt.Uid))
	if user, err := user.LookupId(u); err == nil {
		u = user.Username
	}
	return u, nil
}

func getFileGroup(fi os.FileInfo) (string, error) {
	statt, err := getFileStatt(fi)
	if err != nil {
		return "", nil
	}
	g := strconv.Itoa(int(statt.Gid))
	if group, err := user.LookupGroupId(g); err == nil {
		g = group.Name
	}
	return g, nil
}
