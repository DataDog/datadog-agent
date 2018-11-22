// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package flare

import (
	"fmt"
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

// initPermsInfo creates the permissions.log file and write the headers.
func initPermsInfo(tempDir, hostname string, p os.FileMode) error {
	t := filepath.Join(tempDir, hostname, "permissions.log")

	if err := ensureParentDirsExist(t); err != nil {
		return err
	}

	f, err := os.OpenFile(t, os.O_RDWR|os.O_CREATE|os.O_APPEND, p)
	if err != nil {
		return fmt.Errorf("while opening: %s", err)
	}

	defer f.Close()

	s := fmt.Sprintf("%-50s | %-5s | %-10s | %-10s\n", "File path", "mode", "owner", "group")
	if _, err = f.Write([]byte(s)); err != nil {
		return err
	}

	_, err = f.Write([]byte(strings.Repeat("-", len(s)) + "\n"))
	return err
}

// addPermsInfo appends permissions info of the given file using its filePath
// into the permissions.log file which is shipped in the flare archive.
func addPermsInfo(tempDir, hostname string, p os.FileMode, filePath string) error {
	t := filepath.Join(tempDir, hostname, "permissions.log")
	f, err := os.OpenFile(t, os.O_RDWR|os.O_CREATE|os.O_APPEND, p)
	if err != nil {
		return fmt.Errorf("while opening %s: %s", t, err)
	}

	defer f.Close()

	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("while getting info of %s: %s", filePath, err)
	}

	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		// not enough information to append for this file
		// might happen on system not supporting this feature, but as
		// we're building with !windows tag, shouldn't happen except for
		// very exotic system.
		_, err = f.WriteString(fmt.Sprintf("%-50s | %-5s | %-10s | %-10s\n", filePath, fi.Mode().String(), "???", "???"))
		return err
	}

	u, err := user.LookupId(strconv.Itoa(int(sys.Uid)))
	if err != nil {
		return fmt.Errorf("can't lookup for uid info: %v", err)
	}
	g, err := user.LookupGroupId(strconv.Itoa(int(sys.Gid)))
	if err != nil {
		return fmt.Errorf("can't lookup for gid info: %v", err)
	}

	_, err = f.WriteString(fmt.Sprintf("%-50s | %-5s | %-10s | %-10s\n", filePath, fi.Mode().String(), u.Name, g.Name))
	return err
}
