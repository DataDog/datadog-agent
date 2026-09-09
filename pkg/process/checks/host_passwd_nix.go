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
	"sync"
	"time"
)

const hostPasswdRefreshInterval = time.Second

// hostPasswdCache caches the host user database independently of NSS lookups.
type hostPasswdCache struct {
	mu         sync.Mutex
	now        func() time.Time
	lastCheck  time.Time
	passwdPath string
	passwdInfo os.FileInfo
	users      map[string]user.User
}

func newHostPasswdCache() *hostPasswdCache {
	return &hostPasswdCache{now: time.Now}
}

func (c *hostPasswdCache) lookup(uid string) (*user.User, bool) {
	passwdPath := ""
	if hostEtc := os.Getenv("HOST_ETC"); hostEtc != "" {
		passwdPath = filepath.Join(hostEtc, "passwd")
	}

	c.mu.Lock()
	c.refresh(passwdPath)
	u, found := c.users[uid]
	c.mu.Unlock()

	if !found {
		return nil, false
	}
	return &u, true
}

func (c *hostPasswdCache) refresh(passwdPath string) {
	now := c.now()
	pathChanged := passwdPath != c.passwdPath
	if !pathChanged && !c.lastCheck.IsZero() && now.Sub(c.lastCheck) < hostPasswdRefreshInterval {
		return
	}

	c.lastCheck = now
	if pathChanged {
		c.passwdPath = passwdPath
		c.passwdInfo = nil
		c.users = nil
	}
	if passwdPath == "" {
		return
	}

	info, err := os.Stat(passwdPath)
	if err != nil {
		c.passwdInfo = nil
		c.users = nil
		return
	}
	if c.passwdInfo != nil && os.SameFile(c.passwdInfo, info) && c.passwdInfo.Size() == info.Size() && c.passwdInfo.ModTime().Equal(info.ModTime()) {
		return
	}

	users, err := parsePasswd(passwdPath)
	if err != nil {
		// Keep the last complete snapshot on a transient read failure, but force
		// another parse attempt at the next refresh interval.
		c.passwdInfo = nil
		return
	}
	c.passwdInfo = info
	c.users = users
}

func parsePasswd(path string) (map[string]user.User, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	users := make(map[string]user.User)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 || fields[0] == "" {
			continue
		}
		if _, err := strconv.ParseUint(fields[2], 10, 32); err != nil {
			continue
		}
		if _, found := users[fields[2]]; found {
			continue
		}
		users[fields[2]] = user.User{
			Username: fields[0],
			Uid:      fields[2],
			Gid:      fields[3],
			Name:     fields[4],
			HomeDir:  fields[5],
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
