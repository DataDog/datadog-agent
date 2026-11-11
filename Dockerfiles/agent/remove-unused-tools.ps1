# Removes tools that the base image ships but the Agent never uses, and that vulnerability
# scanners flag:
#   - MinGit, added by the mcr.microsoft.com/dotnet/sdk image (VULN-103527, libssh2).
#   - curl.exe, part of Windows itself, in System32 and the component store (VULN-102629).

# MinGit and its entry in the machine PATH (set by the dotnet/sdk image with setx).
$mingit = 'C:\Program Files\MinGit'
if (Test-Path $mingit) {
    Remove-Item -Recurse -Force $mingit
}
$machinePath = [Environment]::GetEnvironmentVariable('PATH', 'Machine') -split ';' |
    Where-Object { $_ -and ($_ -notlike "$mingit*") }
[Environment]::SetEnvironmentVariable('PATH', ($machinePath -join ';'), 'Machine')

# curl.exe copies are owned by TrustedInstaller, so take ownership before deleting. The
# System32 copy is a hard link to the one in WinSxS, so both must go.
$curls = @()
foreach ($dir in 'C:\Windows\System32', 'C:\Windows\SysWOW64') {
    $curls += Get-Item -Path "$dir\curl.exe" -ErrorAction SilentlyContinue
}
$curls += Get-ChildItem -Path 'C:\Windows\WinSxS' -Directory -Filter '*_curl_*' -ErrorAction SilentlyContinue |
    ForEach-Object { Get-ChildItem -Path $_.FullName -Filter 'curl.exe' -File }
foreach ($curl in $curls) {
    $file = $curl.FullName
    if (Get-Command takeown.exe -ErrorAction SilentlyContinue) {
        takeown.exe /F $file /A | Out-Null
    }
    # S-1-5-32-544 is BUILTIN\Administrators.
    icacls.exe $file /grant '*S-1-5-32-544:F' | Out-Null
    Remove-Item -Force $file
    Write-Host "Removed $file"
}

# Fail the build if anything was left behind.
$left = @()
if (Test-Path $mingit) { $left += $mingit }
$left += Get-ChildItem -Path 'C:\Windows\System32', 'C:\Windows\SysWOW64' -Filter 'curl.exe' -File -ErrorAction SilentlyContinue |
    ForEach-Object FullName
$left += Get-ChildItem -Path 'C:\Windows\WinSxS' -Directory -Filter '*_curl_*' -ErrorAction SilentlyContinue |
    ForEach-Object { Get-ChildItem -Path $_.FullName -Filter 'curl.exe' -File } | ForEach-Object FullName
if ($left) {
    throw "Unused tools still present: $($left -join ', ')"
}
