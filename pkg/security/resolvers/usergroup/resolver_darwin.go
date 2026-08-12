// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usergroup holds usergroup related files
package usergroup

import (
	"os/user"
	"strconv"

	lru "github.com/hashicorp/golang-lru/v2"
)

const resolutionCacheSize = 64

// Resolver resolves macOS user and group names.
//
// The Linux resolver parses /etc/passwd, which on macOS holds only system
// accounts: real users live in Open Directory. os/user goes through getpwuid_r
// in libSystem, which consults it, so no hand-written cgo is needed here.
//
// There is deliberately no container plumbing. The Linux resolver takes a
// container ID and reads that container's /etc/passwd; a developer laptop has no
// such notion.
type Resolver struct {
	usersCache  *lru.Cache[int, string]
	groupsCache *lru.Cache[int, string]
}

// NewResolver returns a new user/group resolver.
func NewResolver() (*Resolver, error) {
	users, err := lru.New[int, string](resolutionCacheSize)
	if err != nil {
		return nil, err
	}
	groups, err := lru.New[int, string](resolutionCacheSize)
	if err != nil {
		return nil, err
	}
	return &Resolver{usersCache: users, groupsCache: groups}, nil
}

// ResolveUser returns the username for a uid.
func (r *Resolver) ResolveUser(uid int) (string, error) {
	if name, found := r.usersCache.Get(uid); found {
		return name, nil
	}

	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", err
	}

	r.usersCache.Add(uid, u.Username)
	return u.Username, nil
}

// ResolveGroup returns the group name for a gid.
func (r *Resolver) ResolveGroup(gid int) (string, error) {
	if name, found := r.groupsCache.Get(gid); found {
		return name, nil
	}

	g, err := user.LookupGroupId(strconv.Itoa(gid))
	if err != nil {
		return "", err
	}

	r.groupsCache.Add(gid, g.Name)
	return g.Name, nil
}
