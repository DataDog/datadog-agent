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

type WindowsPermissionsOption = func(*WindowsPermissions) error

// WindowsPermissions contains the information to configure the permissions of a file on Windows.
type WindowsPermissions struct {
	// DisableInheritance disables the inheritance of permissions from the parent directory.
	DisableInheritance bool

	// If you are familiar with the icacls command, you can provide a custom command directly.
	IcaclsCommand string
}

var _ FilePermissions = (*WindowsPermissions)(nil)

// NewWindowsPermissions creates a new WindowsPermissions object and applies the given options.
func NewWindowsPermissions(options ...WindowsPermissionsOption) option.Option[FilePermissions] {
	p, err := common.ApplyOption(&WindowsPermissions{}, options)

	if err != nil {
		panic("Could not create WindowsPermissions: " + err.Error())
	}
	return option.New[FilePermissions](p)
}

// SetupPermissionsCommand returns a command that sets the permissions of a file. It relies on the icacls command.
func (p *WindowsPermissions) SetupPermissionsCommand(path string) string {
	path = strings.ReplaceAll(path, `/`, `\`) // icacls can behave weirdly with forward slashes
	cmd := ""
	if p.DisableInheritance {
		cmd = fmt.Sprintf(`icacls "%v" /inheritance:r /c /l;`, path)
	}
	if p.IcaclsCommand != "" {
		return fmt.Sprintf(`%v icacls "%v" %v`, cmd, path, p.IcaclsCommand)
	}
	return cmd
}

// ResetPermissionsCommand returns a command that resets the owner, group, and permissions of a file to default.
func (p *WindowsPermissions) ResetPermissionsCommand(path string) string {
	return fmt.Sprintf("icacls “%v” /reset /c /l;", path)
}

// WithIcaclsCommand sets the icacls command to use.
func WithIcaclsCommand(command string) WindowsPermissionsOption {
	return func(p *WindowsPermissions) error {
		p.IcaclsCommand = command
		return nil
	}
}

// WithDisableInheritance disables the inheritance of permissions.
func WithDisableInheritance() WindowsPermissionsOption {
	return func(p *WindowsPermissions) error {
		p.DisableInheritance = true
		return nil
	}
}
