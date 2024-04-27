// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host provides a way to interact with an e2e remote host and capture its state.
package host

import (
	"fmt"
	"io/fs"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"
)

// Host is a remote host environment.
type Host struct {
	t      *testing.T
	remote *components.RemoteHost
	os     e2eos.Descriptor
	arch   e2eos.Architecture
}

// Option is an option to configure a Host.
type Option func(*testing.T, *Host)

// WithDocker installs docker on the host.
func WithDocker() Option {
	return func(t *testing.T, h *Host) {
		installDocker(h.os, h.arch, t, h.remote)
	}
}

// New creates a new Host.
func New(t *testing.T, remote *components.RemoteHost, os e2eos.Descriptor, arch e2eos.Architecture, opts ...Option) *Host {
	host := &Host{
		t:      t,
		remote: remote,
		os:     os,
		arch:   arch,
	}
	for _, opt := range opts {
		opt(t, host)
	}
	return host
}

// State is the state of a remote host.
type State struct {
	Users  []user.User
	Groups []user.Group
	FS     map[string]FileInfo
}

// State returns the state of the host.
func (h *Host) State() State {
	return State{
		Users:  h.users(),
		Groups: h.groups(),
		FS:     h.fs(),
	}
}

func (h *Host) users() []user.User {
	output := h.remote.MustExecute("getent passwd")
	lines := strings.Split(output, "\n")
	var users []user.User
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		require.Len(h.t, parts, 7)
		users = append(users, user.User{
			Username: parts[0],
			Uid:      parts[2],
			Gid:      parts[3],
			Name:     parts[4],
			HomeDir:  parts[5],
		})
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Uid < users[j].Uid
	})
	return users
}

func (h *Host) groups() []user.Group {
	output := h.remote.MustExecute("getent group")
	lines := strings.Split(output, "\n")
	var groups []user.Group
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		require.Len(h.t, parts, 4)
		groups = append(groups, user.Group{
			Name: parts[0],
			Gid:  parts[2],
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Gid < groups[j].Gid
	})
	return groups
}

func (h *Host) fs() map[string]FileInfo {
	ignoreDirs := []string{
		"/proc",
		"/sys",
		"/dev",
		"/run/utmp",
		"/tmp",
	}
	cmd := "find / "
	for _, dir := range ignoreDirs {
		cmd += fmt.Sprintf("-path '%s' -prune -o ", dir)
	}
	cmd += "-printf '%p:%s:%TY-%Tm-%Td %TH:%TM:%TS:%f:%m\\n' 2>/dev/null"
	output := h.remote.MustExecute(cmd)
	lines := strings.Split(output, "\n")

	fileInfos := make(map[string]FileInfo)
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		require.Len(h.t, parts, 6)

		path := parts[0]
		size, _ := strconv.ParseInt(parts[1], 10, 64)
		modTime, _ := time.Parse("2006-01-02 15:04:05", parts[2])
		name := parts[4]
		mode, _ := strconv.ParseUint(parts[5], 10, 32)
		isDir := fs.FileMode(mode).IsDir()

		fileInfos[path] = FileInfo{
			name:    name,
			size:    size,
			mode:    fs.FileMode(mode),
			modTime: modTime,
			isDir:   isDir,
		}
	}
	return fileInfos
}

// FileInfo struct mimics os.FileInfo
type FileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}
