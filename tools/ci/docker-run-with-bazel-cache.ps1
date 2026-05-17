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
& docker run --rm `
    --env=BAZELISK_HOME `
    --env=BUILDBARN_ID_TOKEN `
    --env=CI `
    --env=XDG_CACHE_HOME `
    --volume="${env:XDG_CACHE_HOME}:${env:XDG_CACHE_HOME}" `
    $args
exit $LASTEXITCODE
