# Removes tools that the base image ships but the Agent never uses, and that vulnerability
# scanners flag:
#   - MinGit, added by the mcr.microsoft.com/dotnet/sdk image (VULN-103527, libssh2).
#   - curl.exe, part of Windows itself, in System32 and the component store (VULN-102629).

function Find-Curl {
    Get-ChildItem -Path 'C:\Windows\System32', 'C:\Windows\SysWOW64' -Filter 'curl.exe' -File -ErrorAction SilentlyContinue
    # Component store copies, including the f\ and r\ delta subfolders.
    Get-ChildItem -Path 'C:\Windows\WinSxS' -Filter 'curl.exe' -File -Recurse -Depth 2 -ErrorAction SilentlyContinue
}

# MinGit and its entry in the machine PATH (set by the dotnet/sdk image with setx).
$mingit = 'C:\Program Files\MinGit'
if (Test-Path $mingit) {
    Remove-Item -Recurse -Force $mingit
}
$machinePath = [Environment]::GetEnvironmentVariable('PATH', 'Machine') -split ';' |
    Where-Object { $_ -and ($_ -notlike "$mingit*") }
[Environment]::SetEnvironmentVariable('PATH', ($machinePath -join ';'), 'Machine')

# curl.exe copies are owned by TrustedInstaller, and even ContainerAdministrator may not delete
# them. Taking ownership needs SeTakeOwnershipPrivilege or SeRestorePrivilege, which an admin
# token holds but has disabled. takeown.exe would enable them, but Nano Server does not ship it,
# so enable them in this process and change the owner through .NET.
Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

public static class TokenPrivilege {
    [StructLayout(LayoutKind.Sequential, Pack = 1)]
    struct TOKEN_PRIVILEGES { public int Count; public long Luid; public int Attributes; }

    [DllImport("kernel32.dll")]
    static extern IntPtr GetCurrentProcess();
    [DllImport("advapi32.dll", SetLastError = true)]
    static extern bool OpenProcessToken(IntPtr process, uint access, out IntPtr token);
    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    static extern bool LookupPrivilegeValue(string system, string name, out long luid);
    [DllImport("advapi32.dll", SetLastError = true)]
    static extern bool AdjustTokenPrivileges(IntPtr token, bool disableAll, ref TOKEN_PRIVILEGES state, int length, IntPtr previous, IntPtr returnLength);

    const uint TOKEN_ADJUST_PRIVILEGES = 0x20, TOKEN_QUERY = 0x8;
    const int SE_PRIVILEGE_ENABLED = 2, ERROR_NOT_ALL_ASSIGNED = 1300;

    public static void Enable(string name) {
        IntPtr token;
        if (!OpenProcessToken(GetCurrentProcess(), TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY, out token))
            throw new Win32Exception();
        var state = new TOKEN_PRIVILEGES { Count = 1, Attributes = SE_PRIVILEGE_ENABLED };
        if (!LookupPrivilegeValue(null, name, out state.Luid))
            throw new Win32Exception();
        if (!AdjustTokenPrivileges(token, false, ref state, 0, IntPtr.Zero, IntPtr.Zero))
            throw new Win32Exception();
        if (Marshal.GetLastWin32Error() == ERROR_NOT_ALL_ASSIGNED)
            throw new Exception(name + " is not held by this account");
    }
}
'@

$curls = @(Find-Curl)
if ($curls) {
    # Diagnostics for the build log, in case the steps below fail.
    if (Get-Command whoami.exe -ErrorAction SilentlyContinue) { whoami.exe /priv }
    [TokenPrivilege]::Enable('SeTakeOwnershipPrivilege')
    [TokenPrivilege]::Enable('SeRestorePrivilege')
}
$admins = [System.Security.Principal.SecurityIdentifier]::new('S-1-5-32-544')
foreach ($curl in $curls) {
    $file = $curl.FullName
    # Change only the owner first, then grant access as the new owner.
    $owner = [System.Security.AccessControl.FileSecurity]::new()
    $owner.SetOwner($admins)
    [System.IO.FileSystemAclExtensions]::SetAccessControl([System.IO.FileInfo]::new($file), $owner)
    $acl = [System.IO.FileSystemAclExtensions]::GetAccessControl([System.IO.FileInfo]::new($file))
    $acl.AddAccessRule([System.Security.AccessControl.FileSystemAccessRule]::new($admins, 'FullControl', 'Allow'))
    [System.IO.FileSystemAclExtensions]::SetAccessControl([System.IO.FileInfo]::new($file), $acl)
    Remove-Item -Force $file
    Write-Host "Removed $file"
}

# Fail the build if anything was left behind.
$left = @(Find-Curl | ForEach-Object FullName)
if (Test-Path $mingit) { $left += $mingit }
if ($left) {
    throw "Unused tools still present: $($left -join ', ')"
}
