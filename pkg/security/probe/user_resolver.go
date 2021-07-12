// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"os/user"
	"strconv"

	lru "github.com/hashicorp/golang-lru"
)

// UserGroupResolver resolves user and group ids to names
type UserGroupResolver struct {
	userCache  *lru.Cache
	groupCache *lru.Cache
}

// ResolveUser resolves a user id to a username
func (r *UserGroupResolver) ResolveUser(uid int) (string, error) {
	cachedEntry, found := r.userCache.Get(uid)
	if found {
		return cachedEntry.(string), nil
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
func (r *UserGroupResolver) ResolveGroup(gid int) (string, error) {
	cachedEntry, found := r.groupCache.Get(gid)
	if found {
		return cachedEntry.(string), nil
	}

	var groupname string
	g, err := user.LookupGroupId(strconv.Itoa(gid))
	if err == nil {
		groupname = g.Name
	}
	r.groupCache.Add(gid, groupname)
	return groupname, nil
}

// NewUserGroupResolver instantiates a new user and group resolver
func NewUserGroupResolver() (*UserGroupResolver, error) {
	userCache, err := lru.New(64)
	if err != nil {
		return nil, err
	}

	groupCache, err := lru.New(64)
	if err != nil {
		return nil, err
	}

	return &UserGroupResolver{
		userCache:  userCache,
		groupCache: groupCache,
	}, nil
}
