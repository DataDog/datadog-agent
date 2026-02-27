// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package user offers an interface over user and group management
package user

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"slices"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetGroupID returns the ID of the given group.
func GetGroupID(ctx context.Context, groupName string) (int, error) {
	if groupName == "root" {
		return 0, nil
	}

	if _, err := exec.LookPath("getent"); err == nil {
		cmd := telemetry.CommandContext(ctx, "getent", "group", groupName)
		output, err := cmd.Output()
		if err == nil {
			// Expected output format is groupname:password:gid:users
			parts := strings.Split(strings.TrimSpace(string(output)), ":")
			if len(parts) >= 3 {
				gid, err := strconv.Atoi(parts[2])
				if err == nil {
					return gid, nil
				}
			}
		}
	}

	// Fallback to user package
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
func GetUserID(ctx context.Context, userName string) (int, error) {

	if _, err := exec.LookPath("getent"); err == nil {
		cmd := telemetry.CommandContext(ctx, "getent", "passwd", userName)
		output, err := cmd.Output()
		if err == nil {
			// Expected output format is username:password:uid:gid:gecos:homedir:shell
			parts := strings.Split(strings.TrimSpace(string(output)), ":")
			if len(parts) >= 3 {
				uid, err := strconv.Atoi(parts[2])
				if err == nil {
					return uid, nil
				}
			}
		}
	}

	// Fallback to user package
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
func IsUserInGroup(ctx context.Context, userName, groupName string) (bool, error) {
	if _, err := exec.LookPath("getent"); err == nil {
		groupCmd := telemetry.CommandContext(ctx, "getent", "group", groupName)
		groupOutput, err := groupCmd.Output()
		if err == nil {
			// Expected output format is groupname:password:gid:user1,user2,user3
			groupParts := strings.Split(strings.TrimSpace(string(groupOutput)), ":")
			if len(groupParts) >= 3 {
				groupGid := groupParts[2]

				// Get the user's primary GID
				userCmd := telemetry.CommandContext(ctx, "getent", "passwd", userName)
				userOutput, err := userCmd.Output()
				if err == nil {
					// Expected output format is username:password:uid:gid:gecos:homedir:shell
					userParts := strings.Split(strings.TrimSpace(string(userOutput)), ":")
					if len(userParts) >= 4 {
						userPrimaryGid := userParts[3]
						// Check if the group is the user's primary group
						if userPrimaryGid == groupGid {
							return true, nil
						}
					}
				}

				// Check if user is in the supplementary group members list
				if len(groupParts) >= 4 && groupParts[3] != "" {
					users := strings.SplitSeq(groupParts[3], ",")
					for u := range users {
						if strings.TrimSpace(u) == userName {
							return true, nil
						}
					}
				}
				return false, nil
			}
		}
	}

	// Fallback to user package
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

// EnsureAgentUserAndGroup ensures that the user and group required by the agent are present on the system.
func EnsureAgentUserAndGroup(ctx context.Context, installPath string) error {
	if err := ensureGroup(ctx, "dd-agent"); err != nil {
		return fmt.Errorf("error ensuring dd-agent group: %w", err)
	}
	if err := ensureUser(ctx, "dd-agent", installPath); err != nil {
		return fmt.Errorf("error ensuring dd-agent user: %w", err)
	}
	if err := ensureUserInGroup(ctx, "dd-agent", "dd-agent"); err != nil {
		return fmt.Errorf("error ensuring dd-agent user in dd-agent group: %w", err)
	}
	return nil
}

func ensureGroup(ctx context.Context, groupName string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "ensure_group")
	defer func() {
		span.Finish(err)
	}()
	_, err = GetGroupID(ctx, groupName)
	if err == nil {
		return nil
	}
	var unknownGroupError *user.UnknownGroupError
	if !errors.As(err, &unknownGroupError) {
		log.Warnf("error looking up %s group: %v", groupName, err)
	}
	err = telemetry.CommandContext(ctx, "groupadd", "--force", "--system", groupName).Run()
	if err != nil {
		return fmt.Errorf("error creating %s group: %w", groupName, err)
	}
	return nil
}

func ensureUser(ctx context.Context, userName string, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "ensure_user")
	defer func() {
		span.Finish(err)
	}()
	_, err = GetUserID(ctx, userName)
	if err == nil {
		return nil
	}
	var unknownUserError *user.UnknownUserError
	if !errors.As(err, &unknownUserError) {
		log.Warnf("error looking up %s user: %v", userName, err)
	}
	err = telemetry.CommandContext(ctx, "useradd", "--system", "--shell", "/usr/sbin/nologin", "--home", installPath, "--no-create-home", "--no-user-group", "-g", "dd-agent", "dd-agent").Run()
	if err != nil {
		return fmt.Errorf("error creating %s user: %w", userName, err)
	}
	return nil
}

func ensureUserInGroup(ctx context.Context, userName string, groupName string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "ensure_user_in_group")
	defer func() {
		span.Finish(err)
	}()
	// Check if user is already in group and abort if it is -- this allows us
	// to skip where the user / group are set in LDAP / AD
	userInGroup, err := IsUserInGroup(ctx, userName, groupName)
	if err != nil {
		return fmt.Errorf("error checking if user %s is in group %s: %w", userName, groupName, err)
	}
	if userInGroup {
		return nil
	}
	err = telemetry.CommandContext(ctx, "usermod", "-g", groupName, userName).Run()
	if err != nil {
		return fmt.Errorf("error adding %s user to %s group: %w", userName, groupName, err)
	}
	return nil
}
