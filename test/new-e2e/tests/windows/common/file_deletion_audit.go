// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
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

const globalSACLInterop = `
if (-not ('DatadogGlobalSacl' -as [type])) {
Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;

public static class DatadogGlobalSacl
{
    private const int ERROR_FILE_NOT_FOUND = 2;
    private const UInt32 ACL_REVISION = 2;
    private const int ACL_HEADER_SIZE = 8;
    private const UInt32 TOKEN_QUERY = 0x0008;
    private const UInt32 TOKEN_ADJUST_PRIVILEGES = 0x0020;
    private const UInt32 SE_PRIVILEGE_ENABLED = 0x0002;

    [StructLayout(LayoutKind.Sequential)]
    private struct LUID { public UInt32 LowPart; public Int32 HighPart; }

    [StructLayout(LayoutKind.Sequential)]
    private struct TOKEN_PRIVILEGES { public UInt32 PrivilegeCount; public LUID Luid; public UInt32 Attributes; }

    [DllImport("advapi32.dll", SetLastError = true)]
    private static extern bool OpenProcessToken(IntPtr ProcessHandle, UInt32 DesiredAccess, out IntPtr TokenHandle);

    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool LookupPrivilegeValue(string SystemName, string Name, out LUID Luid);

    [DllImport("advapi32.dll", SetLastError = true)]
    private static extern bool AdjustTokenPrivileges(IntPtr TokenHandle, bool DisableAllPrivileges, ref TOKEN_PRIVILEGES NewState, UInt32 BufferLength, IntPtr PreviousState, IntPtr ReturnLength);

    [return: MarshalAs(UnmanagedType.U1)]
    [DllImport("advapi32.dll", EntryPoint = "AuditQueryGlobalSaclW", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool AuditQueryGlobalSacl(string ObjectTypeName, out IntPtr Acl);

    [return: MarshalAs(UnmanagedType.U1)]
    [DllImport("advapi32.dll", EntryPoint = "AuditSetGlobalSaclW", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool AuditSetGlobalSacl(string ObjectTypeName, IntPtr Acl);

    [return: MarshalAs(UnmanagedType.Bool)]
    [DllImport("advapi32.dll", SetLastError = true)]
    private static extern bool InitializeAcl(IntPtr Acl, UInt32 AclLength, UInt32 AclRevision);

    [DllImport("kernel32.dll")]
    private static extern IntPtr LocalFree(IntPtr Memory);

    [DllImport("kernel32.dll")]
    private static extern IntPtr GetCurrentProcess();

    [DllImport("kernel32.dll")]
    private static extern bool CloseHandle(IntPtr Handle);

    private static void EnableSecurityPrivilege()
    {
        IntPtr token;
        if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY | TOKEN_ADJUST_PRIVILEGES, out token))
            throw new Win32Exception(Marshal.GetLastWin32Error());
        try {
            LUID luid;
            if (!LookupPrivilegeValue(null, "SeSecurityPrivilege", out luid))
                throw new Win32Exception(Marshal.GetLastWin32Error());
            TOKEN_PRIVILEGES privileges = new TOKEN_PRIVILEGES { PrivilegeCount = 1, Luid = luid, Attributes = SE_PRIVILEGE_ENABLED };
            if (!AdjustTokenPrivileges(token, false, ref privileges, 0, IntPtr.Zero, IntPtr.Zero))
                throw new Win32Exception(Marshal.GetLastWin32Error());
            int adjustmentError = Marshal.GetLastWin32Error();
            if (adjustmentError != 0)
                throw new Win32Exception(adjustmentError);
        } finally {
            CloseHandle(token);
        }
    }

    public static byte[] GetFileSacl()
    {
        EnableSecurityPrivilege();
        IntPtr acl;
        if (!AuditQueryGlobalSacl("File", out acl)) {
            int queryError = Marshal.GetLastWin32Error();
            // A host with no global File resource SACL is the normal clean
            // state. Represent it as an empty ACL so SetFileSacl can restore
            // that state with a valid initialized empty ACL.
            if (queryError == ERROR_FILE_NOT_FOUND)
                return new byte[0];
            throw new Win32Exception(queryError);
        }
        if (acl == IntPtr.Zero)
            return new byte[0];
        try {
            int length = (UInt16)Marshal.ReadInt16(acl, 2);
            byte[] result = new byte[length];
            Marshal.Copy(acl, result, 0, length);
            return result;
        } finally {
            LocalFree(acl);
        }
    }

    public static void SetFileSacl(byte[] value)
    {
        EnableSecurityPrivilege();
        IntPtr acl = IntPtr.Zero;
        try {
            int aclLength = value.Length == 0 ? ACL_HEADER_SIZE : value.Length;
            acl = Marshal.AllocHGlobal(aclLength);
            if (value.Length == 0) {
                if (!InitializeAcl(acl, (UInt32)aclLength, ACL_REVISION))
                    throw new Win32Exception(Marshal.GetLastWin32Error());
            } else {
                Marshal.Copy(value, 0, acl, value.Length);
            }
            if (!AuditSetGlobalSacl("File", acl))
                throw new Win32Exception(Marshal.GetLastWin32Error());
        } finally {
            if (acl != IntPtr.Zero)
                Marshal.FreeHGlobal(acl);
        }
    }
}
'@
}
`

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
