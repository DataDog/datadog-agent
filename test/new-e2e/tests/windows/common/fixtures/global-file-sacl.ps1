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
