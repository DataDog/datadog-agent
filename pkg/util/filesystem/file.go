// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strconv"
)

// FileExists returns true if a file exists and is accessible, false otherwise
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadLines reads a file line by line
func ReadLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{""}, err
	}
	defer f.Close()

	var ret []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	return ret, scanner.Err()
}

// ChownDDAgent makes a file owned by the dd-agent user
func ChownDDAgent(path string) error {
	usr, err := user.Lookup("dd-agent")
	if err == nil {
		usrID, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return fmt.Errorf("couldn't parse UID (%s): %w", usr.Uid, err)
		}

		grpID, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return fmt.Errorf("couldn't parse GID (%s): %w", usr.Gid, err)
		}

		if err = os.Chown(path, usrID, grpID); err != nil {
			return fmt.Errorf("couldn't set user and group owner for %s: %w", path, err)
		}
	}

	return nil
}
