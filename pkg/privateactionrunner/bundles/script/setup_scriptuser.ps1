# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2025-present Datadog, Inc.

<#
.SYNOPSIS
    Provisions the dd-scriptuser local account for the Private Action Runner.
.DESCRIPTION
    Creates a low-privilege local user (dd-scriptuser) used by the PAR to
    execute PowerShell scripts. Stores the password in LSA private data,
    grants "Log on as a batch job", and denies interactive logon.
    Idempotent: safe to run multiple times.
.NOTES
    Must be run as Administrator. Requires PowerShell 5.1+.
#>

#Requires -RunAsAdministrator

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ScriptUserName = "dd-scriptuser"
$LSAKey = "L`$datadog_scriptuser_password"
$PasswordLength = 64

function New-RandomPassword {
    param([int]$Length = 64)
    $bytes = New-Object byte[] $Length
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    $rng.GetBytes($bytes)
    return [Convert]::ToBase64String($bytes).Substring(0, $Length)
}

function Set-LsaPrivateData {
    param(
        [string]$Key,
        [string]$Value
    )

    Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;

public class LsaHelper {
    [StructLayout(LayoutKind.Sequential)]
    public struct LSA_UNICODE_STRING {
        public ushort Length;
        public ushort MaximumLength;
        public IntPtr Buffer;
    }

    [StructLayout(LayoutKind.Sequential)]
    public struct LSA_OBJECT_ATTRIBUTES {
        public uint Length;
        public IntPtr RootDirectory;
        public IntPtr ObjectName;
        public uint Attributes;
        public IntPtr SecurityDescriptor;
        public IntPtr SecurityQualityOfService;
    }

    [DllImport("advapi32.dll", SetLastError = true)]
    public static extern uint LsaOpenPolicy(
        IntPtr SystemName,
        ref LSA_OBJECT_ATTRIBUTES ObjectAttributes,
        uint DesiredAccess,
        out IntPtr PolicyHandle);

    [DllImport("advapi32.dll", SetLastError = true)]
    public static extern uint LsaStorePrivateData(
        IntPtr PolicyHandle,
        ref LSA_UNICODE_STRING KeyName,
        ref LSA_UNICODE_STRING PrivateData);

    [DllImport("advapi32.dll", SetLastError = true)]
    public static extern uint LsaClose(IntPtr PolicyHandle);

    [DllImport("advapi32.dll")]
    public static extern int LsaNtStatusToWinError(uint Status);

    public static LSA_UNICODE_STRING ToLsaString(string s) {
        var lsa = new LSA_UNICODE_STRING();
        lsa.Buffer = Marshal.StringToHGlobalUni(s);
        lsa.Length = (ushort)(s.Length * 2);
        lsa.MaximumLength = (ushort)((s.Length + 1) * 2);
        return lsa;
    }

    public static void StorePrivateData(string key, string value) {
        var objAttrs = new LSA_OBJECT_ATTRIBUTES();
        objAttrs.Length = (uint)Marshal.SizeOf(objAttrs);

        IntPtr policyHandle;
        // POLICY_CREATE_SECRET = 0x00000020
        uint result = LsaOpenPolicy(IntPtr.Zero, ref objAttrs, 0x00000020, out policyHandle);
        if (result != 0) {
            throw new Exception("LsaOpenPolicy failed: Win32 error " + LsaNtStatusToWinError(result));
        }
        try {
            var keyStr = ToLsaString(key);
            var valueStr = ToLsaString(value);
            try {
                result = LsaStorePrivateData(policyHandle, ref keyStr, ref valueStr);
                if (result != 0) {
                    throw new Exception("LsaStorePrivateData failed: Win32 error " + LsaNtStatusToWinError(result));
                }
            } finally {
                Marshal.FreeHGlobal(keyStr.Buffer);
                Marshal.FreeHGlobal(valueStr.Buffer);
            }
        } finally {
            LsaClose(policyHandle);
        }
    }
}
"@ -Language CSharp

    [LsaHelper]::StorePrivateData($Key, $Value)
}

function Grant-UserRight {
    param(
        [string]$UserName,
        [string]$Right
    )
    $tempDir = Join-Path $env:TEMP "par-setup-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
    try {
        $cfgFile = Join-Path $tempDir "secpol.cfg"
        $dbFile = Join-Path $tempDir "secpol.sdb"
        secedit /export /cfg $cfgFile /quiet
        $content = Get-Content $cfgFile -Raw
        $account = New-Object System.Security.Principal.NTAccount($UserName)
        $sid = $account.Translate([System.Security.Principal.SecurityIdentifier]).Value

        if ($content -match "(?m)^$Right\s*=\s*(.*)$") {
            $existing = $Matches[1]
            if ($existing -notmatch $sid) {
                $content = $content -replace "(?m)^$Right\s*=\s*(.*)$", "$Right = $existing,*$sid"
            }
        } else {
            $content = $content -replace "(?m)^\[Privilege Rights\]", "[Privilege Rights]`n$Right = *$sid"
        }

        Set-Content -Path $cfgFile -Value $content
        secedit /configure /db $dbFile /cfg $cfgFile /quiet
    } finally {
        Remove-Item -Recurse -Force $tempDir -ErrorAction SilentlyContinue
    }
}

Write-Host "=== Datadog PAR - Script User Provisioning ===" -ForegroundColor Cyan
Write-Host ""

Write-Host "[1/5] Generating password..." -ForegroundColor Yellow
$password = New-RandomPassword -Length $PasswordLength
$securePassword = ConvertTo-SecureString $password -AsPlainText -Force

$existingUser = Get-LocalUser -Name $ScriptUserName -ErrorAction SilentlyContinue
if ($existingUser) {
    Write-Host "[2/5] User '$ScriptUserName' exists, updating password..." -ForegroundColor Yellow
    Set-LocalUser -Name $ScriptUserName -Password $securePassword -PasswordNeverExpires $true
} else {
    Write-Host "[2/5] Creating local user '$ScriptUserName'..." -ForegroundColor Yellow
    New-LocalUser -Name $ScriptUserName `
        -Password $securePassword `
        -FullName "Datadog Script User" `
        -Description "Low-privilege account for PAR script execution" `
        -PasswordNeverExpires `
        -UserMayNotChangePassword `
        -AccountNeverExpires
}

$adminGroup = Get-LocalGroupMember -Group "Administrators" -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -like "*\$ScriptUserName" }
if ($adminGroup) {
    Write-Host "       Removing '$ScriptUserName' from Administrators group..." -ForegroundColor Yellow
    Remove-LocalGroupMember -Group "Administrators" -Member $ScriptUserName
}

Write-Host "[3/5] Storing password in LSA (key: $LSAKey)..." -ForegroundColor Yellow
Set-LsaPrivateData -Key $LSAKey -Value $password

Write-Host "[4/5] Granting SeBatchLogonRight..." -ForegroundColor Yellow
Grant-UserRight -UserName $ScriptUserName -Right "SeBatchLogonRight"

Write-Host "[5/5] Denying SeDenyInteractiveLogonRight..." -ForegroundColor Yellow
Grant-UserRight -UserName $ScriptUserName -Right "SeDenyInteractiveLogonRight"

$password = $null
$securePassword = $null
[System.GC]::Collect()

Write-Host ""
Write-Host "=== Provisioning complete ===" -ForegroundColor Green
Write-Host ""
Write-Host "The '$ScriptUserName' account is ready." -ForegroundColor Green
Write-Host ""
Write-Host "To grant permissions (analogous to sudoers):" -ForegroundColor Cyan
Write-Host "  - Service control:  sc sdset <svcname> <SDDL with $ScriptUserName ACE>" -ForegroundColor White
Write-Host "  - Folder access:    icacls C:\Path /grant ${ScriptUserName}:(OI)(CI)M" -ForegroundColor White
Write-Host "  - Registry access:  Set-Acl on specific registry paths" -ForegroundColor White

