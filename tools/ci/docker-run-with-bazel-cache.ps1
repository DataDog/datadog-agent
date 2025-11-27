$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

# Allow any container user to refresh disk-cache files written by a prior job's container.
$diskCache = Join-Path $env:XDG_CACHE_HOME "bazel\disk-cache"
$null = New-Item $diskCache -ItemType Directory -Force
if (-not (($acl = Get-Acl $diskCache).Access | Where-Object { -not $_.IsInherited -and $_.IdentityReference -eq 'Everyone' })) {
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
        'Everyone', 'FullControl', 'ContainerInherit, ObjectInherit', 'None', 'Allow'))
    Set-Acl $env:XDG_CACHE_HOME $acl
    Set-Acl $diskCache $acl
    Get-ChildItem $diskCache -Recurse | ForEach-Object { Set-Acl $_.FullName $acl }
}
# Temporarily reverted to the pre-fix behavior (C:\bob back on the container's own scratch VHDX, no bind
# mount) so the new disk diagnostics in bazel:test:windows-amd64's POWERSHELL_SCRIPT can capture the
# original disk-full failure if it recurs. Re-apply the $CI_PROJECT_DIR bind mount once we've observed it.
& docker run --rm `
    --storage-opt "size=100GB" `
    --env=BAZELISK_HOME `
    --env=BUILDBARN_ID_TOKEN `
    --env=CI `
    --env=XDG_CACHE_HOME `
    --volume="${env:XDG_CACHE_HOME}:${env:XDG_CACHE_HOME}" `
    $args
exit $LASTEXITCODE
