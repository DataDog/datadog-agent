// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"os/user"
	"strconv"

	"github.com/hashicorp/golang-lru/simplelru"
)

// UserGroupResolver resolves user and group ids to names
type UserGroupResolver struct {
	userCache  *simplelru.LRU
	groupCache *simplelru.LRU
}

// ResolveUser resolves a user id to a username
func (r *UserGroupResolver) ResolveUser(uid int) (string, error) {
	username, found := r.userCache.Get(uid)
	if found {
		return username.(string), nil
	}

	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		r.userCache.Add(uid, "")
		return "", err
	}
	r.userCache.Add(uid, u.Username)

	return u.Username, nil
}

// ResolveGroup resolves a group id to a group name
func (r *UserGroupResolver) ResolveGroup(gid int) (string, error) {
	groupname, found := r.groupCache.Get(gid)
	if found {
		return groupname.(string), nil
	}

	g, err := user.LookupGroupId(strconv.Itoa(gid))
	if err != nil {
		r.groupCache.Add(gid, "")
		return "", err
	}
	r.groupCache.Add(gid, g.Name)

	return g.Name, nil
}

// NewUserGroupResolver instantiates a new user and group resolver
func NewUserGroupResolver() (*UserGroupResolver, error) {
	userCache, err := simplelru.NewLRU(64, nil)
	if err != nil {
		return nil, err
	}

	groupCache, err := simplelru.NewLRU(64, nil)
	if err != nil {
		return nil, err
	}

	return &UserGroupResolver{
		userCache:  userCache,
		groupCache: groupCache,
	}, nil
}
