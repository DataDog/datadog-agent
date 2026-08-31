// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package user offers an interface over user and group management
package user

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"slices"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// agentUser is the macOS service account the Agent runs as. The leading underscore is the
	// platform convention for a system account and matches the account the .dmg has always created.
	agentUser = "_dd-agent"
	// agentGroup is the group the Agent runs in. macOS ships `daemon` on every system, so unlike
	// Linux there is no group to create.
	agentGroup = "daemon"

	// dsclNode is the local directory node. Every dscl call names it explicitly rather than
	// relying on the search path, which may include a network directory the installer must not write to.
	dsclNode = "."

	// agentUIDRange bounds the search for a free UID. macOS reserves 200-400 for system
	// services; Apple's own daemons live at the low end of it.
	agentUIDMin = 300
	agentUIDMax = 400
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

// EnsureAgentUserAndGroup ensures that the user and group required by the agent are present on the system.
//
// On macOS the group already exists, so only the account is created. It is created hidden, with no
// login shell and no home directory of its own, and is idempotent: an account that already exists is
// left exactly as it is, including one provisioned by MDM or a directory service.
func EnsureAgentUserAndGroup(ctx context.Context, installPath string) error {
	if _, err := GetGroupID(ctx, agentGroup); err != nil {
		return fmt.Errorf("error looking up %s group: %w", agentGroup, err)
	}
	if err := ensureUser(ctx, agentUser, installPath); err != nil {
		return fmt.Errorf("error ensuring %s user: %w", agentUser, err)
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
	var unknownUserError user.UnknownUserError
	if !errors.As(err, &unknownUserError) {
		log.Warnf("error looking up %s user: %v", userName, err)
	}

	gid, err := GetGroupID(ctx, agentGroup)
	if err != nil {
		return fmt.Errorf("error looking up %s group: %w", agentGroup, err)
	}
	uid, err := freeUID()
	if err != nil {
		return fmt.Errorf("error finding a free uid: %w", err)
	}

	// dscl has no single "create an account" verb: the record and each of its attributes are
	// separate writes. A failure part-way through leaves an unusable record, so the record is
	// deleted before anything else is attempted, making a retry start from a clean slate.
	if err := dscl(ctx, "-delete", "/Users/"+userName); err != nil {
		log.Debugf("no pre-existing %s record to delete: %v", userName, err)
	}
	writes := [][]string{
		{"-create", "/Users/" + userName},
		{"-create", "/Users/" + userName, "UniqueID", strconv.Itoa(uid)},
		{"-create", "/Users/" + userName, "PrimaryGroupID", strconv.Itoa(gid)},
		{"-create", "/Users/" + userName, "UserShell", "/usr/bin/false"},
		{"-create", "/Users/" + userName, "NFSHomeDirectory", installPath},
		{"-create", "/Users/" + userName, "RealName", "Datadog Agent"},
		{"-create", "/Users/" + userName, "IsHidden", "1"},
	}
	for _, args := range writes {
		if err := dscl(ctx, args...); err != nil {
			return fmt.Errorf("error creating %s user: %w", userName, err)
		}
	}
	return nil
}

// freeUID returns the lowest unused UID in the macOS service range.
func freeUID() (int, error) {
	for uid := agentUIDMin; uid < agentUIDMax; uid++ {
		if _, err := user.LookupId(strconv.Itoa(uid)); err != nil {
			return uid, nil
		}
	}
	return 0, fmt.Errorf("no free uid in the range %d-%d", agentUIDMin, agentUIDMax)
}

func dscl(ctx context.Context, args ...string) error {
	return telemetry.CommandContext(ctx, "/usr/bin/dscl", append([]string{dsclNode}, args...)...).Run()
}
