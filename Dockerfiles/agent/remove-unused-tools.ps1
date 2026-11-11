# Removes tools that the base image ships but the Agent never uses, and that vulnerability
# scanners flag:
#   - MinGit, added by the mcr.microsoft.com/dotnet/sdk image (VULN-103527, libssh2).
#   - curl.exe, part of Windows itself, in System32 and the component store (VULN-102629).

function Invoke-Native {
    & $args[0] $args[1..($args.Length - 1)] | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "'$($args -join ' ')' failed with exit code $LASTEXITCODE"
    }
}

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

# curl.exe copies are owned by TrustedInstaller. The System32 copy is a hard link to the one in
# WinSxS, so both must go. icacls /setowner is used because Nano Server has no takeown.exe.
foreach ($curl in @(Find-Curl)) {
    $file = $curl.FullName
    # S-1-5-32-544 is BUILTIN\Administrators.
    Invoke-Native icacls.exe $file /setowner '*S-1-5-32-544'
    Invoke-Native icacls.exe $file /grant '*S-1-5-32-544:F'
    Remove-Item -Force $file
    Write-Host "Removed $file"
}

# Fail the build if anything was left behind.
$left = @(Find-Curl | ForEach-Object FullName)
if (Test-Path $mingit) { $left += $mingit }
if ($left) {
    throw "Unused tools still present: $($left -join ', ')"
}
