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
	"log/slog"
	"os/exec"
	"os/user"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

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
	_, err = user.LookupGroup(groupName)
	if err == nil {
		return nil
	}
	var unknownGroupError *user.UnknownGroupError
	if !errors.As(err, &unknownGroupError) {
		slog.WarnContext(ctx, "error looking up group", "groupName", groupName, "error", err)
	}
	err = exec.CommandContext(ctx, "groupadd", "--force", "--system", groupName).Run()
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
	_, err = user.Lookup(userName)
	if err == nil {
		return nil
	}
	var unknownUserError *user.UnknownUserError
	if !errors.As(err, &unknownUserError) {
		slog.WarnContext(ctx, "error looking up user", "userName", userName, "error", err)
	}
	err = exec.CommandContext(ctx, "useradd", "--system", "--shell", "/usr/sbin/nologin", "--home", installPath, "--no-create-home", "--no-user-group", "-g", "dd-agent", "dd-agent").Run()
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
	err = exec.CommandContext(ctx, "usermod", "-g", groupName, userName).Run()
	if err != nil {
		return fmt.Errorf("error adding %s user to %s group: %w", userName, groupName, err)
	}
	return nil
}
