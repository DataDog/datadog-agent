// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package filepermissions

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
)

type UnixPermissionsOption = func(*UnixPermissions) error

// UnixPermissions represents the owner, group, and permissions of a file.
// Permissions are represented by a string (should be an octal). There is no check on the octal format.
type UnixPermissions struct {
	Owner       string
	Group       string
	Permissions string
}

var _ FilePermissions = (*UnixPermissions)(nil)

// NewUnixPermissions creates a new UnixPermissions object and applies the given options.
func NewUnixPermissions(options ...UnixPermissionsOption) option.Option[FilePermissions] {
	p, err := common.ApplyOption(&UnixPermissions{}, options)

	if err != nil {
		panic("Could not create UnixPermissions: " + err.Error())
	}
	return option.New[FilePermissions](p)
}

// SetupPermissionsCommand returns a command that sets the owner, group, and permissions of a file.
func (p *UnixPermissions) SetupPermissionsCommand(path string) string {
	var commands []string

	if p.Owner != "" {
		commands = append(commands, fmt.Sprintf(`sudo chown "%s" "%s"`, p.Owner, path))
	}

	if p.Group != "" {
		commands = append(commands, fmt.Sprintf(`sudo chgrp "%s" "%s"`, p.Group, path))
	}

	if p.Permissions != "" {
		commands = append(commands, fmt.Sprintf(`sudo chmod "%s" "%s"`, p.Permissions, path))
	}

	if len(commands) == 0 {
		return ""
	}
	return strings.Join(commands, " && ")
}

// ResetPermissionsCommand returns a command that resets the owner, group, and permissions of a file to default.
func (p *UnixPermissions) ResetPermissionsCommand(path string) string {
	return fmt.Sprintf(`sudo chown "$USER:$(id -gn)" "%s" && sudo chmod 644 "%s"`, path, path)
}

// WithOwner sets the owner of the file.
func WithOwner(owner string) UnixPermissionsOption {
	return func(p *UnixPermissions) error {
		p.Owner = owner
		return nil
	}
}

// WithGroup sets the group of the file.
func WithGroup(group string) UnixPermissionsOption {
	return func(p *UnixPermissions) error {
		p.Group = group
		return nil
	}
}

// WithPermissions sets the permissions of the file.
func WithPermissions(permissions string) UnixPermissionsOption {
	return func(p *UnixPermissions) error {
		p.Permissions = permissions
		return nil
	}
}
