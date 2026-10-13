// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package user offers an interface over user and group management
package user

import (
	"context"
	"fmt"
	"os/user"
	"slices"
	"strconv"
)

// GetGroupID returns the ID of the given group.
//
// macOS has no getent, so this resolves through the directory service via os/user.
func GetGroupID(_ context.Context, groupName string) (int, error) {
	if groupName == "root" {
		return 0, nil
	}
	group, err := user.LookupGroup(groupName)
	if err != nil {
		return 0, err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, fmt.Errorf("error converting gid to int: %w", err)
	}
	return gid, nil
}

// GetUserID returns the ID of the given user.
func GetUserID(_ context.Context, userName string) (int, error) {
	u, err := user.Lookup(userName)
	if err != nil {
		return 0, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, fmt.Errorf("error converting uid to int: %w", err)
	}
	return uid, nil
}

// IsUserInGroup checks if a user is a member of a group.
func IsUserInGroup(_ context.Context, userName, groupName string) (bool, error) {
	group, err := user.LookupGroup(groupName)
	if err != nil {
		return false, err
	}
	u, err := user.Lookup(userName)
	if err != nil {
		return false, err
	}
	userGroups, err := u.GroupIds()
	if err != nil {
		return false, fmt.Errorf("error getting groups for user %s: %w", userName, err)
	}
	return slices.Contains(userGroups, group.Gid), nil
}
