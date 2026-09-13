// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	// WindowsDirectory is the root monitored by FileDeletionAudit.
	WindowsDirectory = `C:\Windows`

	// fileSystemAuditSubcategory is the stable audit-policy GUID for the
	// "File System" subcategory. Using the GUID avoids depending on the host's
	// display language.
	fileSystemAuditSubcategory = "{0CCE921D-69AE-11D9-BED3-505054503030}"
	everyoneSID                = "S-1-1-0"
	deletionAuditLogSize       = 256 * 1024 * 1024
)

// FileDeletionAudit enables successful filesystem-delete auditing below
// C:\Windows and retains enough state to restore the host afterwards.
type FileDeletionAudit struct {
	host                           *components.RemoteHost
	policyBackupPath               string
	originalWindowsSDDL            string
	originalSecurityLogMaximumSize uint64
	securityLogSizeChanged         bool
	restored                       bool
}

// StartFileDeletionAudit saves the current policy and C:\Windows SACL, then
// enables successful filesystem auditing and an inheritable deletion SACL.
// Setup is transactional: any partially applied state is restored on error.
func StartFileDeletionAudit(host *components.RemoteHost) (*FileDeletionAudit, error) {
	audit := &FileDeletionAudit{host: host}

	backupPath, err := GetTemporaryFile(host)
	if err != nil {
		return nil, fmt.Errorf("create audit policy backup path: %w", err)
	}
	audit.policyBackupPath = backupPath
	// auditpol refuses to overwrite an existing file; GetTemporaryFile creates it.
	if err := host.Remove(backupPath); err != nil {
		return nil, fmt.Errorf("prepare audit policy backup path: %w", err)
	}

	if _, err := host.Execute(fmt.Sprintf(`auditpol.exe /backup /file:"%s"`, backupPath)); err != nil {
		_ = host.Remove(backupPath)
		return nil, fmt.Errorf("back up audit policy: %w", err)
	}

	security, err := GetSecurityInfoForPath(host, WindowsDirectory)
	if err != nil {
		restoreErr := audit.restorePolicy()
		return nil, errors.Join(fmt.Errorf("save %s SACL: %w", WindowsDirectory, err), restoreErr)
	}
	audit.originalWindowsSDDL = security.SDDL

	checkpoint, err := GetSecurityLogCheckpoint(host)
	if err != nil {
		restoreErr := audit.Restore()
		return nil, errors.Join(fmt.Errorf("save Security log capacity: %w", err), restoreErr)
	}
	if checkpoint.MaximumSizeInBytes == 0 {
		restoreErr := audit.Restore()
		return nil, errors.Join(errors.New("save Security log capacity: maximum size is zero"), restoreErr)
	}
	audit.originalSecurityLogMaximumSize = checkpoint.MaximumSizeInBytes
	if checkpoint.MaximumSizeInBytes < deletionAuditLogSize {
		// Mark this before applying the setting so a command that partially
		// succeeds and then errors is still rolled back.
		audit.securityLogSizeChanged = true
		if err := setSecurityLogMaximumSize(host, deletionAuditLogSize); err != nil {
			restoreErr := audit.Restore()
			return nil, errors.Join(fmt.Errorf("increase Security log capacity: %w", err), restoreErr)
		}
	}

	if err := audit.EnsureEnabled(); err != nil {
		restoreErr := audit.Restore()
		return nil, errors.Join(err, restoreErr)
	}

	rights := DELETE | FILE_DELETE_CHILD
	inheritance := InheritanceFlagsContainer | InheritanceFlagsObject
	if err := AddFileSystemAuditRule(host, WindowsDirectory, everyoneSID, rights, inheritance, PropagationFlagsNone, AuditFlagsSuccess); err != nil {
		restoreErr := audit.Restore()
		return nil, errors.Join(fmt.Errorf("enable deletion auditing on %s: %w", WindowsDirectory, err), restoreErr)
	}

	return audit, nil
}

// EnsureEnabled idempotently enables successful File System auditing. Calling
// it before every operation also verifies that auditpol can apply the policy.
func (a *FileDeletionAudit) EnsureEnabled() error {
	if a == nil || a.restored {
		return errors.New("file deletion audit is not active")
	}
	if _, err := a.host.Execute(fmt.Sprintf(`auditpol.exe /set /subcategory:"%s" /success:enable`, fileSystemAuditSubcategory)); err != nil {
		return fmt.Errorf("enable successful File System auditing: %w", err)
	}
	return nil
}

// Restore restores the original C:\Windows SACL, audit policy, and Security
// log capacity. It is safe to call repeatedly and attempts every restoration
// even if one fails.
func (a *FileDeletionAudit) Restore() error {
	if a == nil || a.restored {
		return nil
	}

	var saclErr error
	if a.originalWindowsSDDL != "" {
		saclErr = RestoreAuditSecurityInfoForPath(a.host, WindowsDirectory, a.originalWindowsSDDL)
	}
	policyErr := a.restorePolicy()
	logSizeErr := a.restoreSecurityLogMaximumSize()
	if saclErr == nil && policyErr == nil && logSizeErr == nil {
		a.restored = true
	}
	return errors.Join(saclErr, policyErr, logSizeErr)
}

func setSecurityLogMaximumSize(host *components.RemoteHost, size uint64) error {
	if _, err := host.Execute(fmt.Sprintf(`wevtutil.exe set-log Security /ms:%d`, size)); err != nil {
		return fmt.Errorf("set Security log maximum size to %d: %w", size, err)
	}
	return nil
}

func (a *FileDeletionAudit) restoreSecurityLogMaximumSize() error {
	if !a.securityLogSizeChanged {
		return nil
	}
	if err := setSecurityLogMaximumSize(a.host, a.originalSecurityLogMaximumSize); err != nil {
		return fmt.Errorf("restore Security log maximum size: %w", err)
	}
	a.securityLogSizeChanged = false
	return nil
}

func (a *FileDeletionAudit) restorePolicy() error {
	if a.policyBackupPath == "" {
		return nil
	}
	if _, err := a.host.Execute(fmt.Sprintf(`auditpol.exe /restore /file:"%s"`, a.policyBackupPath)); err != nil {
		return fmt.Errorf("restore audit policy: %w", err)
	}
	if err := a.host.Remove(a.policyBackupPath); err != nil {
		return fmt.Errorf("remove audit policy backup: %w", err)
	}
	a.policyBackupPath = ""
	return nil
}
