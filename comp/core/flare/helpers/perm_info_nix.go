// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

// filePermsInfo represents file rights on linux
type filePermsInfo struct {
	path  string
	mode  string
	owner string
	group string
	err   error
}

// Add puts the given filepath in the map
// of files to process later during the commit phase.
func (p permissionsInfos) add(filePath string) {
	info := filePermsInfo{
		path: filePath,
	}
	p[filePath] = &info

	fi, err := os.Stat(filePath)
	if err != nil {
		info.err = fmt.Errorf("could not stat file %s: %s", filePath, err)
		return
	}
	info.mode = fi.Mode().String()

	var sys syscall.Stat_t
	if err := syscall.Stat(filePath, &sys); err != nil {
		info.err = fmt.Errorf("can't retrieve file %s uid/gid infos: %s", filePath, err)
		return
	}

	var uid = strconv.Itoa(int(sys.Uid))
	u, err := user.LookupId(uid)
	if err != nil {
		// User not found, eg: it was deleted from the system
		info.owner = uid
	} else if len(u.Name) > 0 {
		info.owner = u.Name
	} else {
		info.owner = u.Username
	}

	var gid = strconv.Itoa(int(sys.Gid))
	g, err := user.LookupGroupId(gid)
	if err != nil {
		// Group not found, eg: it was deleted from the system
		info.group = gid
	} else {
		info.group = g.Name
	}
}

// Commit resolves the infos of every stacked files in the map
// and then writes the permissions.log file on the filesystem.
func (p permissionsInfos) commit() ([]byte, error) {
	f := &bytes.Buffer{}
	// write headers
	s := fmt.Sprintf("%-50s | %-5s | %-10s | %-10s | %-10s|\n", "File path", "mode", "owner", "group", "error")
	if _, err := f.Write([]byte(s)); err != nil {
		return nil, err
	}
	if _, err := f.Write([]byte(strings.Repeat("-", len(s)) + "\n")); err != nil {
		return nil, err
	}

	// write each file permissions infos
	for _, info := range p {
		infoError := ""
		if info.err != nil {
			infoError = info.err.Error()
		}

		_, err := f.Write([]byte(
			fmt.Sprintf("%-50s | %-5s | %-10s | %-10s | %-10s|\n",
				info.path,
				info.mode,
				info.owner,
				info.group,
				infoError,
			)))
		if err != nil {
			return f.Bytes(), err
		}
	}
	return f.Bytes(), nil
}
