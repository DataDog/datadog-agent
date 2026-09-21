$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

# Despite `FF_USE_WINDOWS_JOB_OBJECT: true` in .gitlab-ci.yml, canceled jobs may leave containers running, causing
# the next job on the same runner to fail with `CreateJvmOutputFile(c:\bob\server\jvm.out) failed: (error: 32):
# The process cannot access the file because it is being used by another process.` as well as less obvious errors:
# [CIEXE-143], [CIEXE-1152]. Since the runner executes only one job per `$CI_PROJECT_DIR`, remove any container
# with a path mounted at or below that directory.
docker ps -aq | ForEach-Object {
    $containerId = $_
    try {
        $mountSources = (docker inspect $containerId | ConvertFrom-Json).Mounts.Source
        Write-Output "Found container $containerId with mounts on: $( $mountSources -join ', ' )"
        if (($mountSources -contains $env:CI_PROJECT_DIR) -or ($mountSources -like "$env:CI_PROJECT_DIR\*")) {
            docker rm -fv $containerId
        }
    } catch {
        Write-Warning "Failed to process container $containerId, it may have already been removed: $_"
    }
}

# Set a job-specific bind mount for Bazel's `outputBase` in order to:
# 1. prevent races on `outputUserRoot\<same workspace hash>\server\jvm.out`,
# 2. avoid heavy I/O on the container's dynamically-expanding + differencing VHDX (`sandbox.vhdx` starts at 41MB),
# 3. use the host's large volume without hitting VHDX expansion limits (`--storage-opt` does not preallocate).
$outputBase = Join-Path $env:CI_PROJECT_DIR ".cache\bob"
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

# The GitLab runner calls `TerminateJobObject` when forcibly canceling a job, which only kills the job's process
# tree (this script's process, the `docker.exe` client, etc.), but not the container because it belongs to the
# Docker service's process tree. A Bazel server still running in there would then prevent the GitLab runner from
# cleaning up the workspace: `failed to remove .cache/bob/execroot/...: Permission denied`.
# To overcome that, we use a watchdog as the container's main process, which:
# 1. waits for the OS to auto-delete a lock file this script creates and holds open, at termination (guaranteed),
# 2. reads then deletes server.pid.txt, which Bazel creates and watches, to interrupt any command then shut down,
# 3. waits for that process to actually exit (Bazel makes it happen within 3 seconds) to avoid breaking caches,
# 4. terminates, letting the Docker service remove the container.
$commandPos = 1
while ($commandPos -le $args.Count -and $args[$commandPos - 1] -notlike 'registry.ddbuild.io/*') {
    $commandPos++
}
if ($commandPos -gt $args.Count) {
    throw "No registry.ddbuild.io image found!"
}
$dockerArgs = @($args | Select-Object -First $commandPos)
$command = @($args | Select-Object -Skip $commandPos)
$lockName = "$env:CI_JOB_ID.lock"
$watchdog = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes(@"
`$ProgressPreference = 'SilentlyContinue'
`$probes = 1
while (`$true) {
    try {
        [IO.File]::Delete('C:\bob\$lockName') # succeeds when already gone
    } catch [IO.IOException] {
        Start-Sleep -Seconds 1
        `$probes++
        continue
    } catch {
        [Console]::Error.WriteLine("unexpected error (`$_) - bailing out")
    }
    break
}
`$serverPid = Get-Content 'C:\bob\server\server.pid.txt' -ErrorAction SilentlyContinue
[Console]::Error.WriteLine("$lockName gone after `$probes probes - terminating Bazel server (`$serverPid)")
if (`$serverPid) {
    Remove-Item 'C:\bob\server\server.pid.txt' -Force
    Wait-Process -Id `$serverPid
}
[Console]::Error.WriteLine("bye")
"@))
$lock = [IO.FileStream]::new((Join-Path $outputBase $lockName), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None, 4096, [IO.FileOptions]::DeleteOnClose)
try {
    $containerId = docker run `
        --detach `
        --env=BAZELISK_HOME `
        --env=BUILDBARN_ID_TOKEN `
        --env=CI `
        --env=XDG_CACHE_HOME `
        --mount="type=bind,src=${outputBase},dst=C:\bob" `
        --mount="type=bind,src=${env:XDG_CACHE_HOME},dst=${env:XDG_CACHE_HOME}" `
        --rm `
        --storage-opt=size=100GB `
        $dockerArgs `
        powershell -NonInteractive -NoLogo -NoProfile -EncodedCommand $watchdog
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    docker exec $containerId $command
    $commandExitCode = $LASTEXITCODE
} finally {
    $lock.Close()
}

$esc = [char]27
$section = "fetch_watchdog_logs"
Write-Output "$($esc)[0Ksection_start:$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()):$($section)[collapsed=true]`r$($esc)[0KFetching watchdog logs"
try {
    docker logs -f $containerId
} catch {
    Write-Output "couldn't fetch watchdog logs - ignoring"
}
Write-Output "$($esc)[0Ksection_end:$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()):$($section)`r$($esc)[0K"
exit $commandExitCode
