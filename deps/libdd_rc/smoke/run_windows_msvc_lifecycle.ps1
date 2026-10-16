$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

[Environment]::SetEnvironmentVariable('RC_X509_LIFECYCLE_SUCCEEDED', $null, 'Process')

$PlatformArgs = @(
    '-c', 'dbg',
    '--platforms=//bazel/platforms:windows_x86_64',
    '--extra_toolchains=@rust_toolchains//:rw-395021384',
    '--extra_toolchains=@winlibs_mingw64//:mingw_cc_toolchain',
    '--action_env=PATH',
    '--@rules_rust//cargo/settings:debug_std_streams_output_group=true'
)
$PackageTarget = '//packages/agent/dependencies:rc_x509_packaged_agent_windows_archive'

$executionRoots = @(bazel info @PlatformArgs execution_root)
if ($LASTEXITCODE -ne 0 -or $executionRoots.Count -ne 1) {
    throw "Could not resolve one Bazel execution root: $executionRoots"
}
$executionRoot = $executionRoots[0]
$env:RC_X509_BAZEL_EXECUTION_ROOT = $executionRoot
Write-Host "Bazel execution root: $executionRoot"

function Resolve-BazelOutputPath {
    param([Parameter(Mandatory = $true)][string]$OutputPath)

    if ([IO.Path]::IsPathRooted($OutputPath)) {
        return $OutputPath
    }
    return (Join-Path $executionRoot $OutputPath)
}

bazel build @PlatformArgs --jobs=8 --verbose_failures --remote_download_outputs=all $PackageTarget
if ($LASTEXITCODE -ne 0) {
    throw "Bazel build failed with exit code $LASTEXITCODE"
}

$packageOutputs = @(bazel cquery @PlatformArgs --output=files $PackageTarget)
if ($LASTEXITCODE -ne 0) {
    throw "Windows package archive cquery failed with exit code $LASTEXITCODE"
}
Write-Host 'Windows package archive declared outputs:'
$packageOutputs | ForEach-Object { Write-Host "  $_" }
$packageArchives = @($packageOutputs | Where-Object { $_ -match '\.tar$' })
if ($packageArchives.Count -ne 1) {
    throw "Expected one Windows package archive, found $($packageArchives.Count): $packageOutputs"
}
$packageArchivePath = Resolve-BazelOutputPath ($packageArchives[0])
if (-not (Test-Path -LiteralPath $packageArchivePath)) {
    throw "Windows package archive does not exist: $packageArchivePath"
}

$stageDir = Join-Path $env:TEMP "rc-x509-msvc-lifecycle-$env:CI_JOB_ID"
if (Test-Path -LiteralPath $stageDir) {
    Remove-Item -LiteralPath $stageDir -Recurse -Force
}
New-Item -Path $stageDir -ItemType Directory | Out-Null
& tar.exe -xf $packageArchivePath -C $stageDir
if ($LASTEXITCODE -ne 0) {
    throw "Could not extract Windows package archive; tar.exe exited with code $LASTEXITCODE"
}

$agentBinDir = Join-Path $stageDir 'bin\agent'
$expectedFiles = @(
    (Join-Path $agentBinDir 'agent.exe'),
    (Join-Path $agentBinDir 'windows_msvc_lifecycle.exe'),
    (Join-Path $agentBinDir 'rc_x509_ffi.dll'),
    (Join-Path $agentBinDir 'aws_lc_fips_0_14_2_crypto.dll')
)
$missingFiles = @($expectedFiles | Where-Object { -not (Test-Path -LiteralPath $_ -PathType Leaf) })
if ($missingFiles.Count -ne 0) {
    throw "Windows package archive is missing expected files: $missingFiles"
}

Write-Host 'Packaged Windows lifecycle probe:'
Get-ChildItem -LiteralPath $agentBinDir | Format-Table -AutoSize
& (Join-Path $agentBinDir 'windows_msvc_lifecycle.exe')
if ($LASTEXITCODE -ne 0) {
    throw "Packaged Windows MSVC/MinGW lifecycle probe exited with code $LASTEXITCODE"
}

Write-Host 'Packaged Windows Agent version probe:'
[Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_ENABLED', 'false', 'Process')
[Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_X509_ENABLED', 'false', 'Process')
& (Join-Path $agentBinDir 'agent.exe') version
if ($LASTEXITCODE -ne 0) {
    throw "Packaged Windows Agent version probe exited with code $LASTEXITCODE"
}

function Get-FreeTcpPort {
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return ([Net.IPEndPoint]$listener.LocalEndpoint).Port
    }
    finally {
        $listener.Stop()
    }
}

function Protect-SensitiveOutput {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Output,
        [string[]]$SensitiveFilePaths = @()
    )

    foreach ($sensitiveFilePath in $SensitiveFilePaths) {
        if (Test-Path -LiteralPath $sensitiveFilePath -PathType Leaf) {
            $sensitiveValue = [string](Get-Content -LiteralPath $sensitiveFilePath -Raw -ErrorAction SilentlyContinue)
            $sensitiveValue = $sensitiveValue.Trim()
            if (-not [string]::IsNullOrWhiteSpace($sensitiveValue)) {
                $Output = $Output -replace [Regex]::Escape($sensitiveValue), '<redacted>'
            }
        }
    }
    return $Output
}

function Get-AgentProbeOutput {
    param(
        [Parameter(Mandatory = $true)][string[]]$Paths,
        [string[]]$SensitiveFilePaths = @()
    )

    $content = @($Paths | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | ForEach-Object {
        Get-Content -LiteralPath $_ -Raw -ErrorAction SilentlyContinue
    })
    return (Protect-SensitiveOutput -Output ($content -join "`n") -SensitiveFilePaths $SensitiveFilePaths)
}

Write-Host 'Packaged Windows Agent x509 component probe:'
$agentConfigDir = Join-Path $stageDir 'config'
New-Item -Path $agentConfigDir -ItemType Directory | Out-Null
$agentLogPath = Join-Path $stageDir 'agent.log'
$agentStdoutPath = Join-Path $stageDir 'agent.stdout.log'
$agentStderrPath = Join-Path $stageDir 'agent.stderr.log'
$agentPidPath = Join-Path $stageDir 'agent.pid'
$authTokenPath = Join-Path $agentConfigDir 'auth_token'
$cmdPort = Get-FreeTcpPort

# YAML accepts forward slashes in Windows paths and avoids backslash escaping.
$agentLogYamlPath = $agentLogPath.Replace('\', '/')
$authTokenYamlPath = $authTokenPath.Replace('\', '/')
$agentConfig = @"
api_key: 00000000000000000000000000000000
hostname: rc-x509-windows-smoke
log_level: debug
log_to_console: true
log_file: '$agentLogYamlPath'
auth_token_file_path: '$authTokenYamlPath'
ipc_address: 127.0.0.1
cmd_port: $cmdPort
use_dogstatsd: false
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: disabled
inventories_enabled: false
enable_metadata_collection: false
remote_configuration:
  enabled: true
  rc_dd_url: http://127.0.0.1:1
  no_tls: true
  x509:
    enabled: true
    debug_ping_enabled: true
"@
Set-Content -LiteralPath (Join-Path $agentConfigDir 'datadog.yaml') -Value $agentConfig -Encoding UTF8

[Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_ENABLED', $null, 'Process')
[Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_X509_ENABLED', $null, 'Process')

$agentProcess = $null
$probeLogPaths = @($agentLogPath, $agentStdoutPath, $agentStderrPath)
try {
    # These tokens are needed by the container/Bazel setup, not by the Agent
    # under test. Keep them out of the child process entirely, and force dummy
    # API-key values so inherited environment overrides cannot inject a real
    # key into this isolated probe.
    $ciIdentityToken = [Environment]::GetEnvironmentVariable('CI_IDENTITIES_GITLAB_ID_TOKEN', 'Process')
    $buildbarnToken = [Environment]::GetEnvironmentVariable('BUILDBARN_ID_TOKEN', 'Process')
    $inheritedAPIKey = [Environment]::GetEnvironmentVariable('DD_API_KEY', 'Process')
    $inheritedRCAPIKey = [Environment]::GetEnvironmentVariable('DD_REMOTE_CONFIGURATION_API_KEY', 'Process')
    try {
        [Environment]::SetEnvironmentVariable('CI_IDENTITIES_GITLAB_ID_TOKEN', $null, 'Process')
        [Environment]::SetEnvironmentVariable('BUILDBARN_ID_TOKEN', $null, 'Process')
        [Environment]::SetEnvironmentVariable('DD_API_KEY', '00000000000000000000000000000000', 'Process')
        [Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_API_KEY', '00000000000000000000000000000000', 'Process')
        $agentProcess = Start-Process `
            -FilePath (Join-Path $agentBinDir 'agent.exe') `
            -ArgumentList @('-c', $agentConfigDir, 'run', '--pidfile', $agentPidPath) `
            -WorkingDirectory $agentBinDir `
            -RedirectStandardOutput $agentStdoutPath `
            -RedirectStandardError $agentStderrPath `
            -PassThru
    }
    finally {
        [Environment]::SetEnvironmentVariable('CI_IDENTITIES_GITLAB_ID_TOKEN', $ciIdentityToken, 'Process')
        [Environment]::SetEnvironmentVariable('BUILDBARN_ID_TOKEN', $buildbarnToken, 'Process')
        [Environment]::SetEnvironmentVariable('DD_API_KEY', $inheritedAPIKey, 'Process')
        [Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_API_KEY', $inheritedRCAPIKey, 'Process')
        $ciIdentityToken = $null
        $buildbarnToken = $null
        $inheritedAPIKey = $null
        $inheritedRCAPIKey = $null
    }

    $startDeadline = [DateTime]::UtcNow.AddSeconds(60)
    $x509Started = $false
    while ([DateTime]::UtcNow -lt $startDeadline -and -not $agentProcess.HasExited) {
        $probeOutput = Get-AgentProbeOutput -Paths $probeLogPaths -SensitiveFilePaths @($authTokenPath)
        if (
            $probeOutput -match 'remote config x509 debug ping handler enabled' -and
            $probeOutput -match 'remote config x509 client started'
        ) {
            $x509Started = $true
            break
        }
        Start-Sleep -Seconds 1
    }
    if (-not $x509Started) {
        $probeOutput = Get-AgentProbeOutput -Paths $probeLogPaths -SensitiveFilePaths @($authTokenPath)
        $agentStatus = if ($agentProcess.HasExited) { "exit code $($agentProcess.ExitCode)" } else { 'still running' }
        throw "Agent did not report that the x509 component started ($agentStatus)`n$probeOutput"
    }

    $stopDeadline = [DateTime]::UtcNow.AddSeconds(30)
    $stopRequested = $false
    $lastStopError = $null
    $previousCertificateValidator = [Net.ServicePointManager]::ServerCertificateValidationCallback
    try {
        [Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
        while ([DateTime]::UtcNow -lt $stopDeadline -and -not $agentProcess.HasExited) {
            if (Test-Path -LiteralPath $authTokenPath -PathType Leaf) {
                $authToken = (Get-Content -LiteralPath $authTokenPath -Raw).Trim()
                try {
                    Invoke-WebRequest `
                        -UseBasicParsing `
                        -Method Post `
                        -Uri "https://127.0.0.1:$cmdPort/agent/stop" `
                        -Headers @{ Authorization = "Bearer $authToken" } | Out-Null
                    $stopRequested = $true
                    break
                }
                catch {
                    # The x509 component can start before the command API is
                    # listening. Retry until the bounded stop deadline.
                    $lastStopError = Protect-SensitiveOutput `
                        -Output ([string]$_.Exception.Message) `
                        -SensitiveFilePaths @($authTokenPath)
                }
            }
            Start-Sleep -Seconds 1
        }
    }
    finally {
        [Net.ServicePointManager]::ServerCertificateValidationCallback = $previousCertificateValidator
    }

    if (-not $stopRequested) {
        $probeOutput = Get-AgentProbeOutput -Paths $probeLogPaths -SensitiveFilePaths @($authTokenPath)
        throw "Could not request a graceful Agent shutdown through the local command API. LastError=$lastStopError`n$probeOutput"
    }
    if (-not $agentProcess.WaitForExit(30000)) {
        $probeOutput = Get-AgentProbeOutput -Paths $probeLogPaths -SensitiveFilePaths @($authTokenPath)
        throw "Agent did not exit within 30 seconds of the graceful stop request`n$probeOutput"
    }
    if ($agentProcess.ExitCode -ne 0) {
        $probeOutput = Get-AgentProbeOutput -Paths $probeLogPaths -SensitiveFilePaths @($authTokenPath)
        throw "Agent x509 component probe exited with code $($agentProcess.ExitCode)`n$probeOutput"
    }

    $probeOutput = Get-AgentProbeOutput -Paths $probeLogPaths -SensitiveFilePaths @($authTokenPath)
    if ($probeOutput -notmatch 'remote config x509 client stopped') {
        throw "Agent exited without reporting a clean x509 component stop`n$probeOutput"
    }
    Write-Host 'Packaged Windows Agent initialized and cleanly stopped the x509 component'
}
finally {
    if ($null -ne $agentProcess -and -not $agentProcess.HasExited) {
        Stop-Process -Id $agentProcess.Id -Force
        $agentProcess.WaitForExit()
    }
    if (Test-Path -LiteralPath $authTokenPath -PathType Leaf) {
        Remove-Item -LiteralPath $authTokenPath -Force -ErrorAction SilentlyContinue
    }
    $authToken = $null
}
[Environment]::SetEnvironmentVariable('RC_X509_LIFECYCLE_SUCCEEDED', '1', 'Process')
