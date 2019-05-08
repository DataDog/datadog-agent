// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package flare

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func zipCounterStrings(tempDir, hostname string) error {
	return nil
}

func zipTypeperfData(tempDir, hostname string) error {
	return nil
}

// Add puts the given filepath in the map
// of files to process later during the commit phase.
func (p permissionsInfos) add(filePath string) {
	p[filePath] = filePermsInfo{}
}

// Commit resolves the infos of every stacked files in the map
// and then writes the permissions.log file on the filesystem.
func (p permissionsInfos) commit(tempDir, hostname string, mode os.FileMode) error {
	if err := p.statFiles(); err != nil {
		return err
	}
	if err := p.write(tempDir, hostname, mode); err != nil {
		return err
	}
	return nil
}

func (p permissionsInfos) statFiles() error {
	for filePath := range p {
		fi, err := os.Stat(filePath)
		if err != nil {
			log.Println(err)
			return fmt.Errorf("while getting info of %s: %s", filePath, err)
		}

		var sys syscall.Stat_t
		if err := syscall.Stat(filePath, &sys); err != nil {
			return fmt.Errorf("can't retrieve file %s uid/gid infos: %s", filePath, err)
		}

		u, err := user.LookupId(strconv.Itoa(int(sys.Uid)))
		if err != nil {
			return fmt.Errorf("can't lookup for uid info: %v", err)
		}
		g, err := user.LookupGroupId(strconv.Itoa(int(sys.Gid)))
		if err != nil {
			return fmt.Errorf("can't lookup for gid info: %v", err)
		}

		uname := u.Name
		if len(uname) == 0 {
			// full name could be empty, use the login name instead
			uname = u.Username
		}

		p[filePath] = filePermsInfo{
			mode:  fi.Mode(),
			owner: uname,
			group: g.Name,
		}
	}
	return nil
}

func (p permissionsInfos) write(tempDir, hostname string, mode os.FileMode) error {
	// init the file
	t := filepath.Join(tempDir, hostname, "permissions.log")

	if err := ensureParentDirsExist(t); err != nil {
		return err
	}

	f, err := os.OpenFile(t, os.O_RDWR|os.O_CREATE|os.O_APPEND, mode)
	if err != nil {
		return fmt.Errorf("while opening: %s", err)
	}

	defer f.Close()

	// write headers
	s := fmt.Sprintf("%-50s | %-5s | %-10s | %-10s\n", "File path", "mode", "owner", "group")
	if _, err = f.Write([]byte(s)); err != nil {
		return err
	}
	if _, err = f.Write([]byte(strings.Repeat("-", len(s)) + "\n")); err != nil {
		return err
	}

	// write each file permissions infos
	for filePath, perms := range p {
		_, err = f.WriteString(fmt.Sprintf("%-50s | %-5s | %-10s | %-10s\n", filePath, perms.mode.String(), perms.owner, perms.group))
		if err != nil {
			return err
		}
	}

	return nil
}
