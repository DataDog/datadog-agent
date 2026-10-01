// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// lookupHostUser returns the first matching host passwd entry, without caching.
// Missing entries and file errors leave resolution to the local user database.
func lookupHostUser(uid string) *user.User {
	hostEtc := os.Getenv("HOST_ETC")
	if hostEtc == "" {
		return nil
	}
	id, err := strconv.ParseUint(uid, 10, 32)
	if err != nil {
		return nil
	}
	file, err := os.Open(filepath.Join(hostEtc, "passwd"))
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok || name == "" {
			continue
		}
		_, rest, ok = strings.Cut(rest, ":")
		if !ok {
			continue
		}
		fileUID, rest, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		entryID, err := strconv.ParseUint(fileUID, 10, 32)
		if err != nil || entryID != id {
			continue
		}
		fields := strings.SplitN(rest, ":", 4)
		if len(fields) < 4 {
			continue
		}
		fullName, _, _ := strings.Cut(fields[1], ",")
		return &user.User{
			Username: name,
			Uid:      strconv.FormatUint(id, 10),
			Gid:      fields[0],
			Name:     fullName,
			HomeDir:  fields[2],
		}
	}
	return nil
}
