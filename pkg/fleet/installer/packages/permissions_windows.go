// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package packages

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows"
)

// fileAllAccess is the Windows FILE_ALL_ACCESS constant (0x1F01FF), equivalent
// to .NET's FileSystemRights.FullControl. It grants read, write, execute,
// delete, and permission-change rights on a file.
const fileAllAccess = 0x1F01FF

// grantDDAgentUserFileAccess adds an explicit FILE_ALL_ACCESS ACE for the core
// Agent service user (ddagentuser) on the given file. Inherited ACEs from the
// parent directory are preserved.
//
// This mirrors the MSI's GrantAgentAccessPermissions (ConfigureUserCustomActions.cs)
// which grants FullControl to files that the Agent service needs to write.
func grantDDAgentUserFileAccess(filePath string) error {
	sid, err := winutil.GetServiceUserSID(coreAgentService)
	if err != nil {
		return fmt.Errorf("could not resolve SID for core Agent user: %w", err)
	}
	if sid == nil {
		return errors.New("core Agent service user SID is nil")
	}

	// LocalSystem already has full control via inheritance; nothing to add.
	lsSid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err == nil && windows.EqualSid(sid, lsSid) {
		return nil
	}

	return paths.AddExplicitAccessToFile(filePath, sid, fileAllAccess)
}

// removeDDAgentUserFileAccess removes explicit ACEs for the core Agent service
// user (ddagentuser) from the given file. Other ACEs (both explicit for other
// SIDs and inherited) are preserved.
//
// This is the inverse of grantDDAgentUserFileAccess and should be called on
// uninstall to clean up explicit permissions added during installation.
func removeDDAgentUserFileAccess(filePath string) error {
	sid, err := winutil.GetServiceUserSID(coreAgentService)
	if err != nil {
		return fmt.Errorf("could not resolve SID for core Agent user: %w", err)
	}
	if sid == nil {
		return errors.New("core Agent service user SID is nil")
	}

	// LocalSystem: we never added an explicit ACE, nothing to remove.
	lsSid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err == nil && windows.EqualSid(sid, lsSid) {
		return nil
	}

	return paths.RevokeExplicitAccessFromFile(filePath, sid)
}
