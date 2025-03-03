// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package user offers an interface over user and group management
package user

import (
	"context"
	"fmt"
	"os/exec"
	"os/user"
)

// EnsureAgentUserAndGroup ensures that the user and group required by the agent are present on the system.
func EnsureAgentUserAndGroup(ctx context.Context) error {
	if _, err := user.LookupGroup("dd-agent"); err == nil {
		return nil
	}
	err := exec.CommandContext(ctx, "groupadd", "--system", "dd-agent").Run()
	if err != nil {
		return fmt.Errorf("error creating dd-agent group: %w", err)
	}
	if _, err := user.Lookup("dd-agent"); err == nil {
		return nil
	}
	err = exec.CommandContext(ctx, "useradd", "--system", "--shell", "/usr/sbin/nologin", "--home", "/opt/datadog-packages", "--no-create-home", "--no-user-group", "-g", "dd-agent", "dd-agent").Run()
	if err != nil {
		return fmt.Errorf("error creating dd-agent user: %w", err)
	}
	err = exec.CommandContext(ctx, "usermod", "-g", "dd-agent", "dd-agent").Run()
	if err != nil {
		return fmt.Errorf("error adding dd-agent user to dd-agent group: %w", err)
	}
	return nil
}
