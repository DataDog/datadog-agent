// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"bufio"
	"bytes"
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

	var uidBuffer [10]byte // Maximum decimal length of a uint32 UID.
	uidBytes := strconv.AppendUint(uidBuffer[:0], id, 10)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] == '#' || line[0] == '+' || line[0] == '-' {
			continue
		}
		name, rest, ok := bytes.Cut(line, []byte(":"))
		if !ok || len(name) == 0 {
			continue
		}
		_, rest, ok = bytes.Cut(rest, []byte(":"))
		if !ok {
			continue
		}
		fileUID, rest, ok := bytes.Cut(rest, []byte(":"))
		if !ok {
			continue
		}
		// Preserve one digit so zero remains distinct from an empty UID.
		for len(fileUID) > 1 && fileUID[0] == '0' {
			fileUID = fileUID[1:]
		}
		if !bytes.Equal(fileUID, uidBytes) {
			continue
		}
		// Copy only the matching line so returned fields own their backing storage.
		entry := string(line)
		fields := strings.SplitN(entry[len(line)-len(rest):], ":", 4)
		if len(fields) < 4 {
			continue
		}
		fullName, _, _ := strings.Cut(fields[1], ",")
		return &user.User{
			Username: entry[:len(name)],
			Uid:      strconv.FormatUint(id, 10),
			Gid:      fields[0],
			Name:     fullName,
			HomeDir:  fields[2],
		}
	}
	return nil
}
