# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

$fixtureRoot = "C:\ddinjector-e2e"
$executable = Join-Path $fixtureRoot "ddinjector-e2e-crash.exe"
$dumpFolder = Join-Path $fixtureRoot "dumps"
$werKey = "HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps\ddinjector-e2e-crash.exe"

New-Item -Path $fixtureRoot -ItemType Directory -Force | Out-Null
New-Item -Path $dumpFolder -ItemType Directory -Force | Out-Null
# Keep this expected dump away from the suite's global C:\dumps diagnostics.
New-Item -Path $werKey -Force | Out-Null
Set-ItemProperty -Path $werKey -Name "DumpFolder" -Value $dumpFolder -Type ExpandString -Force
Set-ItemProperty -Path $werKey -Name "DumpCount" -Value 1 -Type DWord -Force
Set-ItemProperty -Path $werKey -Name "DumpType" -Value 1 -Type DWord -Force

if (-not (Test-Path $executable)) {
    $source = @"
using System;
using System.Runtime.InteropServices;

public static class DDInjectorE2ECrash
{
    [DllImport("kernel32.dll")]
    private static extern void RaiseException(uint code, uint flags, uint argumentCount, IntPtr arguments);

    public static void Main()
    {
        RaiseException(0xc0000005, 1, 0, IntPtr.Zero);
    }
}
"@
    Add-Type -TypeDefinition $source -Language CSharp -OutputAssembly $executable -OutputType ConsoleApplication
}

$process = Start-Process -FilePath $executable -PassThru
$process.WaitForExit()
$unsignedExitStatus = [BitConverter]::ToUInt32([BitConverter]::GetBytes([int]$process.ExitCode), 0)

[PSCustomObject]@{
    process_id = $process.Id
    exit_status = "0x{0:x8}" -f $unsignedExitStatus
} | ConvertTo-Json -Compress

Remove-Item -Path (Join-Path $dumpFolder "*") -Force -ErrorAction SilentlyContinue
Remove-Item -Path $werKey -Recurse -Force -ErrorAction SilentlyContinue
