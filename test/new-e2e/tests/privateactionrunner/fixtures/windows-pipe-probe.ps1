# Starts the PAR executor and inspects its named pipe while it is listening.
# Run as an administrator. Prints one JSON object.
param(
    [Parameter(Mandatory)] [string] $ProcmgrCLI,
    [Parameter(Mandatory)] [string] $ExecutorProcess,
    [Parameter(Mandatory)] [string] $PipePath,
    [Parameter(Mandatory)] [string] $ProbeUser,
    [Parameter(Mandatory)] [string] $ProbePassword,
    [Parameter(Mandatory)] [uint32] $ClientAccess
)
$ErrorActionPreference = 'Stop'

# Compile first: the executor idles out a few seconds after it starts.
Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

public static class ParPipeProbe {
    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    static extern bool LogonUserW(string user, string domain, string password, int logonType, int provider, out IntPtr token);
    [DllImport("advapi32.dll", SetLastError = true)]
    static extern bool ImpersonateLoggedOnUser(IntPtr token);
    [DllImport("advapi32.dll", SetLastError = true)]
    static extern bool RevertToSelf();
    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    static extern IntPtr CreateFileW(string name, uint access, uint share, IntPtr sa, uint disposition, uint flags, IntPtr template);
    [DllImport("kernel32.dll", SetLastError = true)]
    static extern bool CloseHandle(IntPtr handle);
    [DllImport("advapi32.dll")]
    static extern uint GetSecurityInfo(IntPtr handle, int objectType, uint info, IntPtr owner, IntPtr group, IntPtr dacl, IntPtr sacl, out IntPtr sd);
    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    static extern bool ConvertSecurityDescriptorToStringSecurityDescriptorW(IntPtr sd, uint revision, uint info, out IntPtr sddl, IntPtr length);
    [DllImport("kernel32.dll")]
    static extern IntPtr LocalFree(IntPtr memory);

    static readonly IntPtr InvalidHandle = new IntPtr(-1);
    const uint OpenExisting = 3;
    // SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION, as par-control opens the pipe.
    const uint ClientFlags = 0x00110000;
    const uint ReadControl = 0x00020000;
    const int LogonInteractive = 2;
    const int SeKernelObject = 6;
    const uint OwnerAndDacl = 0x1 | 0x4;

    // OpenAs opens the pipe while impersonating user and returns the Win32
    // error, or 0 if the pipe opened.
    public static int OpenAs(string pipe, string user, string password, uint access) {
        IntPtr token;
        if (!LogonUserW(user, ".", password, LogonInteractive, 0, out token)) {
            throw new Win32Exception();
        }
        try {
            if (!ImpersonateLoggedOnUser(token)) {
                throw new Win32Exception();
            }
            try {
                IntPtr handle = CreateFileW(pipe, access, 0, IntPtr.Zero, OpenExisting, ClientFlags, IntPtr.Zero);
                if (handle == InvalidHandle) {
                    return Marshal.GetLastWin32Error();
                }
                CloseHandle(handle);
                return 0;
            } finally {
                RevertToSelf();
            }
        } finally {
            CloseHandle(token);
        }
    }

    // Sddl returns the owner and DACL of the pipe in SDDL form.
    public static string Sddl(string pipe) {
        IntPtr handle = CreateFileW(pipe, ReadControl, 0, IntPtr.Zero, OpenExisting, ClientFlags, IntPtr.Zero);
        if (handle == InvalidHandle) {
            throw new Win32Exception();
        }
        try {
            IntPtr sd;
            uint status = GetSecurityInfo(handle, SeKernelObject, OwnerAndDacl, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, out sd);
            if (status != 0) {
                throw new Win32Exception((int)status);
            }
            try {
                IntPtr sddl;
                if (!ConvertSecurityDescriptorToStringSecurityDescriptorW(sd, 1, OwnerAndDacl, out sddl, IntPtr.Zero)) {
                    throw new Win32Exception();
                }
                try {
                    return Marshal.PtrToStringUni(sddl);
                } finally {
                    LocalFree(sddl);
                }
            } finally {
                LocalFree(sd);
            }
        } finally {
            CloseHandle(handle);
        }
    }
}
'@

function ConvertTo-Sid([string] $account) {
    if ($account -eq 'LocalSystem') { return 'S-1-5-18' }
    if ($account.StartsWith('.\')) { $account = "$env:COMPUTERNAME\" + $account.Substring(2) }
    return (New-Object System.Security.Principal.NTAccount($account)).Translate([System.Security.Principal.SecurityIdentifier]).Value
}

$agentSid = ConvertTo-Sid (Get-CimInstance Win32_Service -Filter "Name='datadogagent'").StartName
$admins = @(Get-LocalGroupMember -SID 'S-1-5-32-544' | ForEach-Object { $_.SID.Value })

# The executor may already be running; a failed start is not an error here.
$ErrorActionPreference = 'Continue'
& $ProcmgrCLI start $ExecutorProcess *> $null
$ErrorActionPreference = 'Stop'

$deadline = (Get-Date).AddSeconds(60)
while (-not ([System.IO.Directory]::GetFiles('\\.\pipe\') -contains $PipePath)) {
    if ((Get-Date) -gt $deadline) { throw "$PipePath did not appear" }
    Start-Sleep -Milliseconds 100
}

$sddl = [ParPipeProbe]::Sddl($PipePath)
$probeError = [ParPipeProbe]::OpenAs($PipePath, $ProbeUser, $ProbePassword, $ClientAccess)

$executor = Get-CimInstance Win32_Process -Filter "Name='privateactionrunner.exe'" |
    Where-Object { $_.CommandLine -like '*run-executor*' } | Select-Object -First 1
$executorSid = ''
if ($executor) {
    $executorSid = (Invoke-CimMethod -InputObject $executor -MethodName GetOwnerSid).Sid
}

$descriptor = New-Object System.Security.AccessControl.RawSecurityDescriptor($sddl)
[pscustomobject]@{
    sddl         = $sddl
    owner        = $descriptor.Owner.Value
    protected    = [bool]($descriptor.ControlFlags -band [System.Security.AccessControl.ControlFlags]::DiscretionaryAclProtected)
    aces         = @($descriptor.DiscretionaryAcl | ForEach-Object {
        [pscustomobject]@{ sid = $_.SecurityIdentifier.Value; mask = [int64]$_.AccessMask; type = [string]$_.AceType }
    })
    probeError   = $probeError
    agentSid     = $agentSid
    agentIsAdmin = $admins -contains $agentSid
    executorSid  = $executorSid
} | ConvertTo-Json -Compress -Depth 4
