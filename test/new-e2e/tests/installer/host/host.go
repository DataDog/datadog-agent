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
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
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
	t      *testing.T
	Users  []user.User
	Groups []user.Group
	FS     map[string]FileInfo
}

// State returns the state of the host.
func (h *Host) State() State {
	return State{
		t:      h.t,
		Users:  h.users(),
		Groups: h.groups(),
		FS:     h.fs(),
	}
}

// Diff is the difference between the current state and another state.
func (h *Host) Diff(other State) Diff {
	return h.State().Diff(other)
}

// Diff returns the difference between two states.
type Diff struct {
	t *testing.T

	UsersAdded           []user.User
	UsersRemoved         []user.User
	usersAddedExpected   map[user.User]struct{}
	usersRemovedExpected map[user.User]struct{}

	GroupsAdded           []user.Group
	GroupsRemoved         []user.Group
	groupsAddedExpected   map[user.Group]struct{}
	groupsRemovedExpected map[user.Group]struct{}

	FSAdded            map[string]FileInfo
	FSRemoved          map[string]FileInfo
	FSModified         map[string]FileInfo
	fsAddedExpected    map[string]struct{}
	fsRemovedExpected  map[string]struct{}
	fsModifiedExpected map[string]struct{}
}

// AssertNoOtherChanges asserts that there are no other changes.
func (s *Diff) AssertNoOtherChanges() {
	assert.Equal(s.t, len(s.UsersAdded), len(s.usersAddedExpected))
	assert.Equal(s.t, len(s.UsersRemoved), len(s.usersRemovedExpected))
	assert.Equal(s.t, len(s.GroupsAdded), len(s.groupsAddedExpected))
	assert.Equal(s.t, len(s.GroupsRemoved), len(s.groupsRemovedExpected))
	assert.Equal(s.t, len(s.FSAdded), len(s.fsAddedExpected))
	assert.Equal(s.t, len(s.FSRemoved), len(s.fsRemovedExpected))
	assert.Equal(s.t, len(s.FSModified), len(s.fsModifiedExpected))
}

// AssertFileCreated asserts that a file was created.
func (s *Diff) AssertFileCreated(path string, mode fs.FileMode, user string, group string) {
	assert.Contains(s.t, s.FSAdded, path)
	assert.Equal(s.t, mode, s.FSAdded[path].mode)
	assert.False(s.t, s.FSAdded[path].isDir)
	assert.Equal(s.t, user, s.FSAdded[path].user)
	assert.Equal(s.t, group, s.FSAdded[path].group)
	s.fsAddedExpected[path] = struct{}{}
}

// AssertDirCreated asserts that a directory was created.
func (s *Diff) AssertDirCreated(path string, mode fs.FileMode, user string, group string) {
	assert.Contains(s.t, s.FSAdded, path)
	assert.Equal(s.t, mode, s.FSAdded[path].mode)
	assert.True(s.t, s.FSAdded[path].isDir)
	assert.Equal(s.t, user, s.FSAdded[path].user)
	assert.Equal(s.t, group, s.FSAdded[path].group)
	s.fsAddedExpected[path] = struct{}{}
}

// AssertUserAdded asserts that a user was added.
func (s *Diff) AssertUserAdded(name string, groupName string) {
	var group user.Group
	for _, g := range s.GroupsAdded {
		if g.Name == groupName {
			group = g
			break
		}
	}
	assert.NotEmpty(s.t, group)
	var found user.User
	for _, u := range s.UsersAdded {
		if u.Username == name && u.Gid == group.Gid {
			found = u
			s.usersAddedExpected[u] = struct{}{}
			break
		}
	}
	assert.NotEmpty(s.t, found)
}

// AssertGroupAdded asserts that a group was added.
func (s *Diff) AssertGroupAdded(name string) {
	var found user.Group
	for _, g := range s.GroupsAdded {
		if g.Name == name {
			found = g
			s.groupsAddedExpected[g] = struct{}{}
			break
		}
	}
	assert.NotEmpty(s.t, found)
}

// Diff returns the difference between two states.
func (s State) Diff(other State) Diff {
	diff := Diff{
		t:                     s.t,
		FSAdded:               make(map[string]FileInfo),
		FSRemoved:             make(map[string]FileInfo),
		FSModified:            make(map[string]FileInfo),
		usersAddedExpected:    make(map[user.User]struct{}),
		usersRemovedExpected:  make(map[user.User]struct{}),
		groupsAddedExpected:   make(map[user.Group]struct{}),
		groupsRemovedExpected: make(map[user.Group]struct{}),
		fsAddedExpected:       make(map[string]struct{}),
		fsRemovedExpected:     make(map[string]struct{}),
		fsModifiedExpected:    make(map[string]struct{}),
	}
	for path, info := range s.FS {
		if _, ok := other.FS[path]; !ok {
			diff.FSAdded[path] = info
		} else if other.FS[path] != info {
			diff.FSModified[path] = info
		}
	}
	for path, info := range other.FS {
		if _, ok := s.FS[path]; !ok {
			diff.FSRemoved[path] = info
		}
	}
	for _, user := range s.Users {
		if !slices.Contains(other.Users, user) {
			diff.UsersRemoved = append(diff.UsersRemoved, user)
		}
	}
	for _, user := range other.Users {
		if !slices.Contains(s.Users, user) {
			diff.UsersAdded = append(diff.UsersAdded, user)
		}
	}
	for _, group := range s.Groups {
		if !slices.Contains(other.Groups, group) {
			diff.GroupsRemoved = append(diff.GroupsRemoved, group)
		}
	}
	for _, group := range other.Groups {
		if !slices.Contains(s.Groups, group) {
			diff.GroupsAdded = append(diff.GroupsAdded, group)
		}
	}
	return diff
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
		"/opt/datadog-packages",
	}
	cmd := "find / "
	for _, dir := range ignoreDirs {
		cmd += fmt.Sprintf("-path '%s' -prune -o ", dir)
	}
	cmd += "-printf '%p:%s:%TY-%Tm-%Td %TH:%TM:%TS:%f:%m:%u:%g\\n' 2>/dev/null"
	output := h.remote.MustExecute(cmd)
	lines := strings.Split(output, "\n")

	fileInfos := make(map[string]FileInfo)
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		require.Len(h.t, parts, 8)

		path := parts[0]
		size, _ := strconv.ParseInt(parts[1], 10, 64)
		modTime, _ := time.Parse("2006-01-02 15:04:05", parts[2])
		name := parts[4]
		mode, _ := strconv.ParseUint(parts[5], 10, 32)
		user := parts[6]
		group := parts[7]
		isDir := fs.FileMode(mode).IsDir()

		fileInfos[path] = FileInfo{
			name:    name,
			size:    size,
			mode:    fs.FileMode(mode),
			modTime: modTime,
			isDir:   isDir,
			user:    user,
			group:   group,
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
	user    string
	group   string
}
