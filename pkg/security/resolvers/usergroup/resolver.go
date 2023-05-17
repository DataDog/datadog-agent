// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package usergroup

import (
	"os/user"
	"strconv"

	lru "github.com/hashicorp/golang-lru/v2"
)

// Resolver resolves user and group ids to names
type Resolver struct {
	userCache  *lru.Cache[int, string]
	groupCache *lru.Cache[int, string]
}

// ResolveUser resolves a user id to a username
func (r *Resolver) ResolveUser(uid int) (string, error) {
	cachedEntry, found := r.userCache.Get(uid)
	if found {
		return cachedEntry, nil
	}

	var username string
	u, err := user.LookupId(strconv.Itoa(uid))
	if err == nil {
		username = u.Username
	}
	r.userCache.Add(uid, username)
	return username, err
}

// ResolveGroup resolves a group id to a group name
func (r *Resolver) ResolveGroup(gid int) (string, error) {
	cachedEntry, found := r.groupCache.Get(gid)
	if found {
		return cachedEntry, nil
	}

	var groupname string
	g, err := user.LookupGroupId(strconv.Itoa(gid))
	if err == nil {
		groupname = g.Name
	}
	r.groupCache.Add(gid, groupname)
	return groupname, nil
}

// NewResolver instantiates a new user and group resolver
func NewResolver() (*Resolver, error) {
	userCache, err := lru.New[int, string](64)
	if err != nil {
		return nil, err
	}

	groupCache, err := lru.New[int, string](64)
	if err != nil {
		return nil, err
	}

	return &Resolver{
		userCache:  userCache,
		groupCache: groupCache,
	}, nil
}
