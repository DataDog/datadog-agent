$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

# Set a job-specific bind mount for Bazel's `outputBase` in order to:
# 1. prevent races on `outputUserRoot\<same workspace hash>\server\jvm.out`,
# 2. avoid heavy I/O on the container's dynamically-expanding + differencing VHDX (`sandbox.vhdx` starts at 41MB),
# 3. use the host's large volume without hitting VHDX expansion limits (`--storage-opt` does not preallocate).
$outputBase = Join-Path $env:CI_PROJECT_DIR ".cache\bob" # $CI_PROJECT_DIR is unique per slot and swept at startup
$null = New-Item $outputBase -ItemType Directory -Force

# Despite `FF_USE_WINDOWS_JOB_OBJECT: true` in .gitlab-ci.yml, canceled jobs may leave containers running, causing
# the next job on the same runner to fail with: `CreateJvmOutputFile(c:\bob\server\jvm.out) failed: (error: 32):
# The process cannot access the file because it is being used by another process.` (CIEXE-1152). Since the runner
# executes only one job per CI_PROJECT_DIR, remove any such container having the same Bazel's `outputBase` mounted.
docker ps -aq | ForEach-Object {
    $containerId = $_
    try {
        $container = docker inspect $containerId | ConvertFrom-Json
        Write-Output "Found container $containerId with mounts on: $($container.Mounts.Source -join ', ')"
        if ($container.Mounts.Source -contains $outputBase) {
            docker rm -fv $containerId
        }
    } catch {
        Write-Warning "Failed to process container $containerId, it may have already been removed: $_"
    }
}

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
docker run --rm `
    --env=BAZELISK_HOME `
    --env=BUILDBARN_ID_TOKEN `
    --env=CI `
    --env=XDG_CACHE_HOME `
    --mount="type=bind,src=${outputBase},dst=C:\bob" `
    --mount="type=bind,src=${env:XDG_CACHE_HOME},dst=${env:XDG_CACHE_HOME}" `
    --storage-opt=size=100GB `
    $args
exit $LASTEXITCODE
