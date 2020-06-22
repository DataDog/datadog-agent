// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package checks

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func getFileOwner(fi os.FileInfo) (string, error) {
	if statt, ok := fi.Sys().(*syscall.Stat_t); ok {
		var (
			u = strconv.Itoa(int(statt.Gid))
			g = strconv.Itoa(int(statt.Uid))
		)
		if group, err := user.LookupGroupId(g); err == nil {
			g = group.Name
		}
		if user, err := user.LookupId(u); err == nil {
			u = user.Username
		}
		return fmt.Sprintf("%s:%s", u, g), nil
	}
	return "", errors.New("expected to get stat_t from fileinfo")
}
