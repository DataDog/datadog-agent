// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package userresolver resolves host user IDs without masking users from the
// local system's user database.
package userresolver

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

const defaultRefreshInterval = time.Second

// LookupIDFunc has the same signature as user.LookupId.
type LookupIDFunc func(string) (*user.User, error)

// Resolver resolves UIDs from HOST_ETC/passwd before using its fallback.
type Resolver struct {
	fallback LookupIDFunc

	mu              sync.Mutex
	now             func() time.Time
	refreshInterval time.Duration
	lastCheck       time.Time
	checked         bool
	passwdPath      string
	passwdInfo      os.FileInfo
	users           map[string]*user.User
}

// New returns a host-aware UID resolver. fallback must not be nil.
func New(fallback LookupIDFunc) *Resolver {
	return &Resolver{
		fallback:        fallback,
		now:             time.Now,
		refreshInterval: defaultRefreshInterval,
	}
}

// LookupID resolves uid from HOST_ETC/passwd when HOST_ETC is set and falls
// back to the local system user database otherwise.
func (r *Resolver) LookupID(uid string) (*user.User, error) {
	hostEtc := os.Getenv("HOST_ETC")
	passwdPath := ""
	if hostEtc != "" {
		passwdPath = filepath.Join(hostEtc, "passwd")
	}

	r.mu.Lock()
	r.refresh(passwdPath)
	hostUser := r.users[uid]
	r.mu.Unlock()

	if hostUser != nil {
		copy := *hostUser
		return &copy, nil
	}
	return r.fallback(uid)
}

func (r *Resolver) refresh(passwdPath string) {
	now := r.now()
	pathChanged := passwdPath != r.passwdPath
	if !pathChanged && r.checked && now.Sub(r.lastCheck) < r.refreshInterval {
		return
	}

	r.checked = true
	r.lastCheck = now
	if pathChanged {
		r.passwdPath = passwdPath
		r.passwdInfo = nil
		r.users = nil
	}
	if passwdPath == "" {
		return
	}

	info, err := os.Stat(passwdPath)
	if err != nil {
		r.passwdInfo = nil
		r.users = nil
		return
	}
	if r.passwdInfo != nil && os.SameFile(r.passwdInfo, info) && r.passwdInfo.Size() == info.Size() && r.passwdInfo.ModTime().Equal(info.ModTime()) {
		return
	}

	users, err := parsePasswd(passwdPath)
	if err != nil {
		// Keep the last complete snapshot on a transient read failure, but force
		// another parse attempt at the next refresh interval.
		r.passwdInfo = nil
		return
	}
	r.passwdInfo = info
	r.users = users
}

func parsePasswd(path string) (map[string]*user.User, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	users := make(map[string]*user.User)
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
		users[fields[2]] = &user.User{
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
