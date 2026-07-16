$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

# Set a job-specific bind mount for Bazel's `outputBase` in order to:
# 1. prevent races on `outputUserRoot\<same workspace hash>\server\jvm.out`,
# 2. avoid heavy I/O on the container's dynamically-expanding + differencing VHDX (`sandbox.vhdx` starts at 41MB),
# 3. use the host's large volume without hitting VHDX expansion limits (`--storage-opt` does not preallocate).
$outputBase = Join-Path $env:CI_PROJECT_DIR ".cache\bob" # $CI_PROJECT_DIR is unique per slot and swept at startup
$null = New-Item $outputBase -ItemType Directory -Force

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
    --storage-opt "size=100GB" `
    --env=BAZELISK_HOME `
    --env=BUILDBARN_ID_TOKEN `
    --env=CI `
    --env=XDG_CACHE_HOME `
    --volume="${outputBase}:C:\bob" `
    --volume="${env:XDG_CACHE_HOME}:${env:XDG_CACHE_HOME}" `
    $args
exit $LASTEXITCODE
