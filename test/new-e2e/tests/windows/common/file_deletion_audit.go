// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	// WindowsDirectory is the root monitored by FileDeletionAudit.
	WindowsDirectory = `C:\Windows`

	// fileSystemAuditSubcategory is the stable audit-policy GUID for the
	// "File System" subcategory. Using the GUID avoids depending on the host's
	// display language.
	fileSystemAuditSubcategory = "{0CCE921D-69AE-11D9-BED3-505054503030}"
	everyoneSID                = "{S-1-1-0}"
	deletionAuditLogSize       = 256 * 1024 * 1024
)

// FileDeletionAudit enables successful global filesystem-delete auditing;
// consumers filter events to C:\Windows. It retains enough state to restore
// the host afterwards.
type FileDeletionAudit struct {
	host                           *components.RemoteHost
	policyBackupPath               string
	originalGlobalFileSACL         string
	globalFileSACLChanged          bool
	originalSecurityLogMaximumSize uint64
	securityLogSizeChanged         bool
	restored                       bool
}

// StartFileDeletionAudit saves the current policy and global File resource
// SACL, then enables successful filesystem deletion auditing. Setup is
// transactional: any partially applied state is restored on error.
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

	audit.originalGlobalFileSACL, err = getGlobalFileResourceSACL(host)
	if err != nil {
		restoreErr := audit.restorePolicy()
		return nil, errors.Join(fmt.Errorf("save global File resource SACL: %w", err), restoreErr)
	}

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

	// Mark this before applying the resource SACL so partial success is restored.
	audit.globalFileSACLChanged = true
	if err := audit.EnsureEnabled(); err != nil {
		restoreErr := audit.Restore()
		return nil, errors.Join(err, restoreErr)
	}

	return audit, nil
}

// IsActive reports whether the audit lifecycle can record another operation.
func (a *FileDeletionAudit) IsActive() bool {
	return a != nil && !a.restored
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
	// Quote the SID so PowerShell passes the curly braces literally instead of
	// parsing them as a script block before invoking auditpol.
	cmd := fmt.Sprintf(`auditpol.exe /resourceSACL /set /type:File /user:"%s" /success /access:0x10040`, everyoneSID)
	if _, err := a.host.Execute(cmd); err != nil {
		return fmt.Errorf("enable global File deletion resource SACL: %w", err)
	}
	return nil
}

// Restore restores the original global File resource SACL, audit policy, and
// Security log capacity. It is safe to call repeatedly and attempts every
// restoration even if one fails.
func (a *FileDeletionAudit) Restore() error {
	if a == nil || a.restored {
		return nil
	}

	saclErr := a.restoreGlobalFileResourceSACL()
	policyErr := a.restorePolicy()
	logSizeErr := a.restoreSecurityLogMaximumSize()
	if saclErr == nil && policyErr == nil && logSizeErr == nil {
		a.restored = true
	}
	return errors.Join(saclErr, policyErr, logSizeErr)
}

//go:embed fixtures/global-file-sacl.ps1
var globalSACLInterop string

func getGlobalFileResourceSACL(host *components.RemoteHost) (string, error) {
	out, err := host.Execute(globalSACLInterop + `[Convert]::ToBase64String([DatadogGlobalSacl]::GetFileSacl())`)
	if err != nil {
		return "", fmt.Errorf("query global File resource SACL: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func (a *FileDeletionAudit) restoreGlobalFileResourceSACL() error {
	if !a.globalFileSACLChanged {
		return nil
	}
	cmd := fmt.Sprintf(`%s [DatadogGlobalSacl]::SetFileSacl([Convert]::FromBase64String('%s'))`, globalSACLInterop, a.originalGlobalFileSACL)
	if _, err := a.host.Execute(cmd); err != nil {
		return fmt.Errorf("restore global File resource SACL: %w", err)
	}
	a.globalFileSACLChanged = false
	return nil
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
